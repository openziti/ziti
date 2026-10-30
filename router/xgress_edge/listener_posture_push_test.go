package xgress_edge

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	sdkedge "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/posture"
	"github.com/openziti/ziti/v2/router/state"
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

// pushTestStateManager serves the RDM and posture data a posture state write reads. Any other
// state.Manager method panics through the nil embedded interface.
type pushTestStateManager struct {
	state.Manager
	rdm  atomic.Pointer[common.RouterDataModel]
	data atomic.Pointer[posture.InstanceData]
}

func (self *pushTestStateManager) RouterDataModel() *common.RouterDataModel {
	return self.rdm.Load()
}

func (self *pushTestStateManager) GetPostureData(string) *posture.InstanceData {
	return self.data.Load()
}

func newPushTestConn(ch channel.Channel) *edgeClientConn {
	stateManager := &pushTestStateManager{}
	stateManager.rdm.Store(common.NewBareRouterDataModel(""))
	return newPushTestConnWithState(ch, stateManager)
}

func newPushTestConnWithState(ch channel.Channel, stateManager state.Manager) *edgeClientConn {
	conn := &edgeClientConn{
		ch:       sdkedge.NewSingleSdkChannel(ch),
		listener: &listener{factory: &Factory{stateManager: stateManager}},
		apiSessionToken: &state.ApiSessionToken{
			ApiSession: &edge_ctrl_pb.ApiSession{Id: "api-session-1", IdentityId: "identity1"},
		},
	}
	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.Unlock()
	return conn
}

func emitTestPostureState(conn *edgeClientConn) {
	conn.emitPostureStateChange()
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
	req.True(pending, "a further write must be owed")

	testCh.releaseSends()

	// exactly two writes: the one that was in flight, and the coalesced newest
	req.Eventually(func() bool {
		conn.posture.send.Lock()
		defer conn.posture.send.Unlock()
		return !conn.posture.send.writing && !conn.posture.send.pending
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
		return !conn.posture.send.writing && !conn.posture.send.pending
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

// Test_PostureStatePush_WritesStateCurrentWhenWritten covers pushes queued out of order. Posture
// evaluations for one api session run concurrently, so one that read older posture can queue its
// push after one that read newer. The write reads the state as it builds the message, so the last
// message carries the current state whatever order the pushes were queued in.
func Test_PostureStatePush_WritesStateCurrentWhenWritten(t *testing.T) {
	req := require.New(t)

	failing := &posture.InstanceData{Macs: &edge_client_pb.PostureResponse_Macs{Addresses: []string{"66:77:88:99:aa:bb"}}}
	passing := &posture.InstanceData{Macs: &edge_client_pb.PostureResponse_Macs{Addresses: []string{"00:11:22:33:44:55"}}}

	stateManager := &pushTestStateManager{}
	stateManager.rdm.Store(newPostureStateTestRdm(req))
	stateManager.data.Store(failing)

	testCh := newBlockingTestChannel(t)
	conn := newPushTestConnWithState(testCh, stateManager)

	emitTestPostureState(conn)
	testCh.awaitSendEntered(req)

	// queued while the failing state is current, then overtaken by an update before it is written
	emitTestPostureState(conn)
	stateManager.data.Store(passing)

	testCh.releaseSends()

	req.Eventually(func() bool {
		conn.posture.send.Lock()
		defer conn.posture.send.Unlock()
		return !conn.posture.send.writing && !conn.posture.send.pending
	}, 5*time.Second, time.Millisecond)
	req.Equal(2, len(testCh.sent))

	req.False(postureCheckPassing(req, decodePostureStateChange(req, <-testCh.sent), "chk-1"))
	req.True(postureCheckPassing(req, decodePostureStateChange(req, <-testCh.sent), "chk-1"),
		"the last write must carry the state current when it was written, not when it was queued")
}

// newPostureStateTestRdm extends newPostureDefTestRdm with a dial policy that grants identity1 and
// requires the MAC check chk-1, so the posture state built for identity1 reports on chk-1.
func newPostureStateTestRdm(req *require.Assertions) *common.RouterDataModel {
	rdm, _ := newPostureDefTestRdm(req)
	rdm.HandleServicePolicyEvent(8,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_ServicePolicy{ServicePolicy: &edge_ctrl_pb.DataState_ServicePolicy{
			Id:         "sp-1",
			Name:       "sp-1",
			PolicyType: edge_ctrl_pb.PolicyType_DialPolicy,
		}})
	rdm.HandleServicePolicyChange(9, &edge_ctrl_pb.DataState_ServicePolicyChange{
		PolicyId:          "sp-1",
		RelatedEntityIds:  []string{"chk-1"},
		RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
		Add:               true,
	})
	rdm.HandleServicePolicyChange(10, &edge_ctrl_pb.DataState_ServicePolicyChange{
		PolicyId:          "sp-1",
		RelatedEntityIds:  []string{"identity1"},
		RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
		Add:               true,
	})
	rdm.SetCurrentIndex(10)
	return rdm
}

func postureCheckPassing(req *require.Assertions, ps *edge_client_pb.PostureStateChange, checkId string) bool {
	for _, cs := range ps.CheckStates {
		if cs.CheckId == checkId {
			return cs.IsPassing
		}
	}
	req.Failf("check state missing", "no state for check %s", checkId)
	return false
}

func decodePostureStateChange(req *require.Assertions, msg *channel.Message) *edge_client_pb.PostureStateChange {
	ps := &edge_client_pb.PostureStateChange{}
	req.NoError(proto.Unmarshal(msg.Body, ps))
	return ps
}
