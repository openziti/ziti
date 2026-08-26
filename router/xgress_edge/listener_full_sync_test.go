package xgress_edge

import (
	"testing"

	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	sdkedge "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/state"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// recordingTestChannel captures sent messages for assertions; everything else is a no-op.
type recordingTestChannel struct {
	NoopTestChannel
	sent []channel.Sendable
}

func (self *recordingTestChannel) Send(s channel.Sendable) error {
	self.sent = append(self.sent, s)
	return nil
}

func newFullSyncTestConn(ch *recordingTestChannel) *edgeClientConn {
	return &edgeClientConn{
		ch: sdkedge.NewSingleSdkChannel(ch),
		apiSessionToken: &state.ApiSessionToken{
			ApiSession: &edge_ctrl_pb.ApiSession{IdentityId: "identity1"},
		},
	}
}

// Test_SendFullServiceSync_InactiveIsNoOp ensures an IdentityFullState notification on a
// connection with no push subscription does nothing (the common case: every non-subscribed
// connection's RDM listener receives the identity's arrival).
func Test_SendFullServiceSync_InactiveIsNoOp(t *testing.T) {
	conn := &edgeClientConn{}

	conn.sendFullServiceSync()

	conn.svcSubscription.Lock()
	defer conn.svcSubscription.Unlock()
	require.False(t, conn.svcSubscription.fullSyncPending)
}

// Test_SendFullServiceSync_DeferredWhileSnapshotPending ensures a full sync arriving while a
// subscribe snapshot is in flight is deferred to the subscribe path via fullSyncPending instead
// of being sent concurrently (the snapshot send happens outside structuralSendMu, so sending here
// could put the higher-index full sync on the wire before the snapshot, whose full reset would
// then regress the SDK's view).
func Test_SendFullServiceSync_DeferredWhileSnapshotPending(t *testing.T) {
	conn := &edgeClientConn{}
	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.snapshotPending = true
	conn.svcSubscription.Unlock()

	conn.sendFullServiceSync()

	conn.svcSubscription.Lock()
	defer conn.svcSubscription.Unlock()
	require.True(t, conn.svcSubscription.fullSyncPending,
		"a full sync racing an in-flight subscribe snapshot must defer to the subscribe path")
}

// Test_SendFullSyncLocked_SendsFullResetAndAdvancesIndex ensures the identity-arrival full sync
// goes out as an authoritative full reset (previousIndex -1) at the current RDM index and
// advances the connection's structural index chain.
func Test_SendFullSyncLocked_SendsFullResetAndAdvancesIndex(t *testing.T) {
	req := require.New(t)

	testCh := &recordingTestChannel{}
	conn := newFullSyncTestConn(testCh)
	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.lastIndex = 2
	conn.svcSubscription.Unlock()

	rdm := common.NewBareRouterDataModel("r1")
	rdm.HandleIdentityEvent(5,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_Identity{Identity: &edge_ctrl_pb.DataState_Identity{Id: "identity1", Name: "identity1"}})
	rdm.SetCurrentIndex(5)

	req.True(conn.sendFullSyncLocked(rdm))

	req.Len(testCh.sent, 1)
	msg, ok := testCh.sent[0].(*channel.Message)
	req.True(ok)
	req.Equal(int32(sdkedge.ContentTypeServiceChangeSet), msg.ContentType)

	cs := &edge_client_pb.ServiceChangeSet{}
	req.NoError(proto.Unmarshal(msg.Body, cs))
	req.Equal(int64(5), cs.Index)
	req.Equal(int64(-1), cs.PreviousIndex, "identity-arrival sync must be an authoritative full reset")

	conn.svcSubscription.Lock()
	defer conn.svcSubscription.Unlock()
	req.Equal(int64(5), conn.svcSubscription.lastIndex)
}

// Test_SendFullSyncLocked_DropsCoveredIndex ensures a full sync at or below the last emitted
// index (e.g. a duplicate IdentityFullState notification) is dropped rather than re-sent, so the
// index chain never regresses.
func Test_SendFullSyncLocked_DropsCoveredIndex(t *testing.T) {
	req := require.New(t)

	testCh := &recordingTestChannel{}
	conn := newFullSyncTestConn(testCh)
	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.lastIndex = 5
	conn.svcSubscription.Unlock()

	rdm := common.NewBareRouterDataModel("r1")
	rdm.SetCurrentIndex(5)

	req.False(conn.sendFullSyncLocked(rdm))
	req.Empty(testCh.sent)

	conn.svcSubscription.Lock()
	defer conn.svcSubscription.Unlock()
	req.Equal(int64(5), conn.svcSubscription.lastIndex)
}
