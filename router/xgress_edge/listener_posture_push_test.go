/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

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

func Test_PostureStatePush(t *testing.T) {
	// Emission is called from the connection read loop and from the shared posture evaluation pool,
	// so it must not park on a peer that has stopped reading. Parking a pool worker would stall
	// posture enforcement for unrelated identities.
	t.Run("emitting does not block on a peer that is not reading", func(t *testing.T) {
		req := require.New(t)

		// arrange
		testCh := newBlockingTestChannel(t)
		conn := newPushTestConn(testCh, newPushTestStateManager(common.NewBareRouterDataModel(""), nil))
		done := make(chan struct{}, 1)

		// act
		go emitPostureStates(conn, 20, done)

		// assert
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			req.Fail("emitting posture state blocked on a peer that is not reading")
		}
	})

	// The message is a full state replacement, so only the newest is worth sending: while one write
	// is in flight, further changes replace each other in a single slot rather than piling up.
	t.Run("changes queued while a write is blocked coalesce into one write", func(t *testing.T) {
		req := require.New(t)

		// arrange: the first emit starts the writer; wait until it is inside Send, so the changes
		// below can't coalesce with the state it is already carrying
		testCh := newBlockingTestChannel(t)
		conn := newPushTestConn(testCh, newPushTestStateManager(common.NewBareRouterDataModel(""), nil))
		conn.emitPostureStateChange()
		select {
		case <-testCh.entered:
		case <-time.After(5 * time.Second):
			req.FailNow("the writer never entered Send")
		}

		// act
		for range 50 {
			conn.emitPostureStateChange()
		}

		// assert
		conn.posture.send.Lock()
		pending := conn.posture.send.pending
		conn.posture.send.Unlock()
		req.True(pending, "a further write must be owed")

		testCh.releaseSends()
		requirePostureWriterIdle(t, conn)
		req.Len(testCh.sent, 2, "the write in flight plus one coalesced write of the 50 changes queued behind it")
	})

	// The writer releases its slot under the same lock producers queue on, so a change arriving
	// after it drains starts a new writer instead of being left unsent.
	t.Run("a change after the writer drains starts a new writer", func(t *testing.T) {
		req := require.New(t)

		// arrange: sends never block
		testCh := newBlockingTestChannel(t)
		testCh.releaseSends()
		conn := newPushTestConn(testCh, newPushTestStateManager(common.NewBareRouterDataModel(""), nil))

		// act
		for range 100 {
			conn.emitPostureStateChange()
			requirePostureWriterIdle(t, conn)
		}

		// assert
		req.Len(testCh.sent, 100, "every change must be written when nothing blocks")
	})

	// The SDK reads a gap in sequence numbers as lost state and answers with a resync request.
	t.Run("coalescing leaves no gaps in sequence numbers", func(t *testing.T) {
		req := require.New(t)

		// arrange
		testCh := newBlockingTestChannel(t)
		testCh.releaseSends()
		conn := newPushTestConn(testCh, newPushTestStateManager(common.NewBareRouterDataModel(""), nil))
		done := make(chan struct{}, 8)

		// act
		for range 8 {
			go emitPostureStates(conn, 25, done)
		}
		for range 8 {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				req.FailNow("emitting posture state did not return")
			}
		}
		requirePostureWriterIdle(t, conn)

		// assert
		seqs := map[uint64]struct{}{}
		for len(testCh.sent) > 0 {
			msg := <-testCh.sent
			ps := &edge_client_pb.PostureStateChange{}
			req.NoError(proto.Unmarshal(msg.Body, ps))
			_, dup := seqs[ps.Seq]
			req.False(dup, "seq %d written twice", ps.Seq)
			seqs[ps.Seq] = struct{}{}
		}
		for seq := uint64(1); seq <= uint64(len(seqs)); seq++ {
			req.Contains(seqs, seq, "seq %d missing, the SDK reads that as a gap", seq)
		}
	})

	// Posture evaluations for one api session run concurrently, so one that read older posture can
	// queue its push after one that read newer. The writer reads the state as it builds the message,
	// so the last message carries the current state whatever order the pushes were queued in.
	t.Run("the last write carries the state current when it is written", func(t *testing.T) {
		req := require.New(t)

		// arrange: a write of the failing state is in flight
		failing := &posture.InstanceData{Macs: &edge_client_pb.PostureResponse_Macs{Addresses: []string{"66:77:88:99:aa:bb"}}}
		passing := &posture.InstanceData{Macs: &edge_client_pb.PostureResponse_Macs{Addresses: []string{"00:11:22:33:44:55"}}}
		stateManager := newPushTestStateManager(newPostureStateTestRdm(), failing)
		testCh := newBlockingTestChannel(t)
		conn := newPushTestConn(testCh, stateManager)
		conn.emitPostureStateChange()
		select {
		case <-testCh.entered:
		case <-time.After(5 * time.Second):
			req.FailNow("the writer never entered Send")
		}

		// act: queue a push while the failing state is current, then update the state before it is
		// written
		conn.emitPostureStateChange()
		stateManager.data.Store(passing)
		testCh.releaseSends()

		// assert
		requirePostureWriterIdle(t, conn)
		req.Len(testCh.sent, 2)
		first := &edge_client_pb.PostureStateChange{}
		req.NoError(proto.Unmarshal((<-testCh.sent).Body, first))
		last := &edge_client_pb.PostureStateChange{}
		req.NoError(proto.Unmarshal((<-testCh.sent).Body, last))
		req.Len(first.CheckStates, 1, "identity1 has one posture check")
		req.Equal("chk-1", first.CheckStates[0].CheckId)
		req.False(first.CheckStates[0].IsPassing, "the first write carries the failing state it was started with")
		req.Len(last.CheckStates, 1, "identity1 has one posture check")
		req.Equal("chk-1", last.CheckStates[0].CheckId)
		req.True(last.CheckStates[0].IsPassing,
			"the last write must carry the state current when it was written, not when it was queued")
	})
}

// requirePostureWriterIdle fails the test unless conn's posture writer has no write in flight and
// none owed within five seconds.
func requirePostureWriterIdle(t *testing.T, conn *edgeClientConn) {
	t.Helper()
	require.Eventually(t, func() bool {
		conn.posture.send.Lock()
		defer conn.posture.send.Unlock()
		return !conn.posture.send.writing && !conn.posture.send.pending
	}, 5*time.Second, time.Millisecond, "the posture writer did not go idle")
}

// emitPostureStates emits count posture state changes on conn, then signals done.
func emitPostureStates(conn *edgeClientConn, count int, done chan<- struct{}) {
	for range count {
		conn.emitPostureStateChange()
	}
	done <- struct{}{}
}

// blockingTestChannel holds each Send until released, standing in for an SDK that has stopped
// draining its channel. It signals entered before blocking, so a test can wait for the writer to
// actually be inside Send rather than inferring it from producer-side state.
type blockingTestChannel struct {
	NoopTestChannel
	entered     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	sent        chan *channel.Message
}

// newBlockingTestChannel returns a blockingTestChannel whose sends are released when the test
// ends, so a test that fails before releasing them doesn't leave the writer parked.
func newBlockingTestChannel(t *testing.T) *blockingTestChannel {
	t.Helper()

	result := &blockingTestChannel{
		entered: make(chan struct{}, 4096),
		release: make(chan struct{}),
		sent:    make(chan *channel.Message, 4096),
	}
	t.Cleanup(result.releaseSends)
	return result
}

// releaseSends lets every blocked and future Send complete.
func (self *blockingTestChannel) releaseSends() {
	self.releaseOnce.Do(func() { close(self.release) })
}

func (self *blockingTestChannel) Send(s channel.Sendable) error {
	self.entered <- struct{}{}
	<-self.release
	self.sent <- s.Msg()
	return nil
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

// newPushTestStateManager returns a pushTestStateManager serving rdm and data. data may be nil.
func newPushTestStateManager(rdm *common.RouterDataModel, data *posture.InstanceData) *pushTestStateManager {
	result := &pushTestStateManager{}
	result.rdm.Store(rdm)
	result.data.Store(data)
	return result
}

func (self *pushTestStateManager) RouterDataModel() *common.RouterDataModel {
	return self.rdm.Load()
}

func (self *pushTestStateManager) GetPostureData(string) *posture.InstanceData {
	return self.data.Load()
}

// newPushTestConn returns a connection for identity1 with an active service subscription, sending
// on ch and reading state from stateManager.
func newPushTestConn(ch channel.Channel, stateManager state.Manager) *edgeClientConn {
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

// newPostureStateTestRdm returns a router data model in which identity1 is granted a dial policy
// requiring the MAC check chk-1, which passes only for 00:11:22:33:44:55, so the posture state
// built for identity1 reports on chk-1 alone.
func newPostureStateTestRdm() *common.RouterDataModel {
	rdm := common.NewBareRouterDataModel("r1")
	rdm.HandleIdentityEvent(1,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_Identity{Identity: &edge_ctrl_pb.DataState_Identity{Id: "identity1", Name: "identity1"}})
	rdm.HandlePostureCheckEvent(2,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_PostureCheck{PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			Id:     "chk-1",
			Name:   "mac-check",
			TypeId: "MAC",
			Subtype: &edge_ctrl_pb.DataState_PostureCheck_Mac_{
				Mac: &edge_ctrl_pb.DataState_PostureCheck_Mac{MacAddresses: []string{"00:11:22:33:44:55"}},
			},
		}})
	rdm.HandleServicePolicyEvent(3,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_ServicePolicy{ServicePolicy: &edge_ctrl_pb.DataState_ServicePolicy{
			Id:         "sp-1",
			Name:       "sp-1",
			PolicyType: edge_ctrl_pb.PolicyType_DialPolicy,
		}})
	rdm.HandleServicePolicyChange(4, &edge_ctrl_pb.DataState_ServicePolicyChange{
		PolicyId:          "sp-1",
		RelatedEntityIds:  []string{"chk-1"},
		RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
		Add:               true,
	})
	rdm.HandleServicePolicyChange(5, &edge_ctrl_pb.DataState_ServicePolicyChange{
		PolicyId:          "sp-1",
		RelatedEntityIds:  []string{"identity1"},
		RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
		Add:               true,
	})
	rdm.SetCurrentIndex(5)
	return rdm
}
