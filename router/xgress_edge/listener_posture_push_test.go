package xgress_edge

import (
	"sync"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	sdkedge "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// blockingTestChannel holds each Send until released, standing in for an SDK that has stopped
// draining its channel. It signals entry on entered before blocking, so a test can wait for the
// writer to actually be inside Send rather than inferring it from producer-side state.
type blockingTestChannel struct {
	NoopTestChannel
	entered     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	sent        chan *channel.Message
}

func newBlockingTestChannel(t *testing.T) *blockingTestChannel {
	result := &blockingTestChannel{
		entered: make(chan struct{}, 4096),
		release: make(chan struct{}),
		sent:    make(chan *channel.Message, 4096),
	}
	// a test that fails before releasing must not leave the writer parked
	t.Cleanup(result.releaseSends)
	return result
}

func (self *blockingTestChannel) releaseSends() {
	self.releaseOnce.Do(func() { close(self.release) })
}

func (self *blockingTestChannel) Send(s channel.Sendable) error {
	self.entered <- struct{}{}
	<-self.release
	self.sent <- s.Msg()
	return nil
}

// awaitSendEntered blocks until a writer has entered Send.
func (self *blockingTestChannel) awaitSendEntered(req *require.Assertions) {
	select {
	case <-self.entered:
	case <-time.After(5 * time.Second):
		req.Fail("no writer entered Send")
	}
}

func (self *blockingTestChannel) IsClosed() bool {
	return false
}

func (self *blockingTestChannel) ConnectionId() string {
	return "blocking-test-conn"
}

func newPushTestConn(ch channel.Channel) *edgeClientConn {
	conn := &edgeClientConn{ch: sdkedge.NewSingleSdkChannel(ch)}
	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.Unlock()
	return conn
}

func emitTestPostureState(conn *edgeClientConn) {
	conn.emitPostureStateChange(common.NewBareRouterDataModel(""), 1, nil)
}

// Test_PostureStatePush_DoesNotBlockCaller is the core of the fix: emission is called from the
// connection read loop and from the shared posture evaluation pool, so it must not park on a peer
// that has stopped reading. Parking a pool worker would stall posture enforcement for unrelated
// identities.
func Test_PostureStatePush_DoesNotBlockCaller(t *testing.T) {
	req := require.New(t)

	testCh := newBlockingTestChannel(t)
	conn := newPushTestConn(testCh)

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		for range 20 {
			emitTestPostureState(conn)
		}
	}()

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		req.Fail("emitting posture state blocked on a peer that is not reading")
	}

	testCh.releaseSends()
}

// Test_PostureStatePush_CoalescesWhileBlocked covers the queue being a single slot: while one
// write is in flight, further state changes replace each other rather than piling up, because the
// message is a full state replacement and only the newest is worth sending.
func Test_PostureStatePush_CoalescesWhileBlocked(t *testing.T) {
	req := require.New(t)

	testCh := newBlockingTestChannel(t)
	conn := newPushTestConn(testCh)

	// the first emit starts the writer; wait until it is actually inside Send, so the replacements
	// below cannot coalesce with the state it is already carrying
	emitTestPostureState(conn)
	testCh.awaitSendEntered(req)

	for range 50 {
		emitTestPostureState(conn)
	}

	conn.posture.send.Lock()
	pending := conn.posture.send.pending
	conn.posture.send.Unlock()
	req.NotNil(pending, "the newest state must be queued")

	testCh.releaseSends()

	// exactly two writes: the one that was in flight, and the coalesced newest
	req.Eventually(func() bool {
		conn.posture.send.Lock()
		defer conn.posture.send.Unlock()
		return !conn.posture.send.writing && conn.posture.send.pending == nil
	}, 5*time.Second, time.Millisecond)

	req.Equal(2, len(testCh.sent), "50 queued state changes must coalesce into one write")
}

// Test_PostureStatePush_WriterRestartsAfterDraining covers the exit-and-restart handoff: the
// writer releases its slot under the same lock producers queue on, so a state change arriving
// after it drains starts a new writer instead of being left unsent.
func Test_PostureStatePush_WriterRestartsAfterDraining(t *testing.T) {
	req := require.New(t)

	testCh := newBlockingTestChannel(t)
	testCh.releaseSends() // never block
	conn := newPushTestConn(testCh)

	for range 100 {
		emitTestPostureState(conn)
		req.Eventually(func() bool {
			conn.posture.send.Lock()
			defer conn.posture.send.Unlock()
			return !conn.posture.send.writing
		}, 5*time.Second, time.Millisecond, "writer must exit once the queue drains")
	}

	req.Equal(100, len(testCh.sent), "every state change must be written when nothing blocks")
}

// Test_PostureStatePush_SeqIsContiguous locks in that coalescing does not skip sequence numbers.
// The SDK reads a gap as lost state and answers with a resync request.
func Test_PostureStatePush_SeqIsContiguous(t *testing.T) {
	req := require.New(t)

	testCh := newBlockingTestChannel(t)
	testCh.releaseSends()
	conn := newPushTestConn(testCh)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 25 {
				emitTestPostureState(conn)
			}
		}()
	}
	wg.Wait()

	req.Eventually(func() bool {
		conn.posture.send.Lock()
		defer conn.posture.send.Unlock()
		return !conn.posture.send.writing && conn.posture.send.pending == nil
	}, 5*time.Second, time.Millisecond)

	seqs := map[uint64]struct{}{}
	for len(testCh.sent) > 0 {
		msg := <-testCh.sent
		ps := decodePostureStateChange(req, msg)
		_, dup := seqs[ps.Seq]
		req.False(dup, "seq %d written twice", ps.Seq)
		seqs[ps.Seq] = struct{}{}
	}

	// whatever coalescing did, the seqs written are 1..n with no gaps
	for seq := uint64(1); seq <= uint64(len(seqs)); seq++ {
		req.Contains(seqs, seq, "seq %d missing, the SDK reads that as a gap", seq)
	}
}

func decodePostureStateChange(req *require.Assertions, msg *channel.Message) *edge_client_pb.PostureStateChange {
	ps := &edge_client_pb.PostureStateChange{}
	req.NoError(proto.Unmarshal(msg.Body, ps))
	return ps
}
