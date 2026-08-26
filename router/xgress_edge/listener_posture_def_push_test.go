package xgress_edge

import (
	"testing"

	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	sdkedge "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func (self *recordingTestChannel) ConnectionId() string {
	return "test-conn"
}

func newPostureDefTestRdm(req *require.Assertions) (*common.RouterDataModel, *common.Identity) {
	rdm := common.NewBareRouterDataModel("r1")
	rdm.HandleIdentityEvent(1,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_Identity{Identity: &edge_ctrl_pb.DataState_Identity{Id: "identity1", Name: "identity1"}})
	rdm.HandlePostureCheckEvent(6,
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create},
		&edge_ctrl_pb.DataState_Event_PostureCheck{PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			Id:     "chk-1",
			Name:   "mac-check",
			TypeId: "MAC",
			Subtype: &edge_ctrl_pb.DataState_PostureCheck_Mac_{
				Mac: &edge_ctrl_pb.DataState_PostureCheck_Mac{MacAddresses: []string{"00:11:22:33:44:55"}},
			},
		}})
	rdm.SetCurrentIndex(7)

	identity, ok := rdm.Identities.Get("identity1")
	req.True(ok)
	return rdm, identity
}

// Test_PostureCheckDefinitionChangeIsPushed covers B1's core: a posture-check DEFINITION change
// produces no service events (membership is unchanged), so it must ride the scan pass's envelope
// as its own PostureCheckDef entry — one incremental ServiceChangeSet with only the changed def,
// chained on the connection's index.
func Test_PostureCheckDefinitionChangeIsPushed(t *testing.T) {
	req := require.New(t)

	testCh := &recordingTestChannel{}
	conn := &edgeClientConn{ch: sdkedge.NewSingleSdkChannel(testCh)}
	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.lastIndex = 5
	conn.svcSubscription.Unlock()

	rdm, identity := newPostureDefTestRdm(req)

	state := &common.IdentityState{
		Identity:             identity,
		ChangedPostureChecks: map[string]common.PostureCheckChangeType{"chk-1": common.PostureCheckUpdated},
	}
	conn.NotifyIdentityEvent(state, common.IdentityPostureChecksUpdatedEvent)
	conn.NotifyBatchComplete(rdm, 7)

	req.Len(testCh.sent, 1)
	msg, ok := testCh.sent[0].(*channel.Message)
	req.True(ok)
	req.Equal(int32(sdkedge.ContentTypeServiceChangeSet), msg.ContentType)

	cs := &edge_client_pb.ServiceChangeSet{}
	req.NoError(proto.Unmarshal(msg.Body, cs))
	req.Equal(int64(7), cs.Index)
	req.Equal(int64(5), cs.PreviousIndex)
	req.Empty(cs.Services, "a pure definition change carries no service entries")
	req.Empty(cs.Policies, "a pure definition change carries no policy entries")
	req.Len(cs.PostureChecks, 1)
	req.Equal("chk-1", cs.PostureChecks[0].Id)
	req.Equal(edge_client_pb.Op_Updated, cs.PostureChecks[0].Op)
	req.Equal("MAC", cs.PostureChecks[0].Type)
}

// Test_PostureCheckRemovalIsPushedIdOnly ensures a check that is no longer applicable (or was
// deleted) ships as an id-only removed entry so the SDK can prune its cached def.
func Test_PostureCheckRemovalIsPushedIdOnly(t *testing.T) {
	req := require.New(t)

	testCh := &recordingTestChannel{}
	conn := &edgeClientConn{ch: sdkedge.NewSingleSdkChannel(testCh)}
	conn.svcSubscription.Lock()
	conn.svcSubscription.active = true
	conn.svcSubscription.lastIndex = 5
	conn.svcSubscription.Unlock()

	rdm, identity := newPostureDefTestRdm(req)

	// chk-gone is not in the RDM at build time — deleted, or removed from the identity's view.
	state := &common.IdentityState{
		Identity:             identity,
		ChangedPostureChecks: map[string]common.PostureCheckChangeType{"chk-gone": common.PostureCheckRemoved},
	}
	conn.NotifyIdentityEvent(state, common.IdentityPostureChecksUpdatedEvent)
	conn.NotifyBatchComplete(rdm, 7)

	req.Len(testCh.sent, 1)
	msg, ok := testCh.sent[0].(*channel.Message)
	req.True(ok)

	cs := &edge_client_pb.ServiceChangeSet{}
	req.NoError(proto.Unmarshal(msg.Body, cs))
	req.Len(cs.PostureChecks, 1)
	req.Equal("chk-gone", cs.PostureChecks[0].Id)
	req.Equal(edge_client_pb.Op_Removed, cs.PostureChecks[0].Op)
	req.Empty(cs.PostureChecks[0].Type, "removed entries are id-only")
}

// Test_PostureCheckChangeNotBufferedWhenInactive ensures a definition change on a connection with
// no push subscription buffers nothing and sends nothing.
func Test_PostureCheckChangeNotBufferedWhenInactive(t *testing.T) {
	req := require.New(t)

	testCh := &recordingTestChannel{}
	conn := &edgeClientConn{ch: sdkedge.NewSingleSdkChannel(testCh)}

	rdm, identity := newPostureDefTestRdm(req)

	state := &common.IdentityState{
		Identity:             identity,
		ChangedPostureChecks: map[string]common.PostureCheckChangeType{"chk-1": common.PostureCheckUpdated},
	}
	conn.NotifyIdentityEvent(state, common.IdentityPostureChecksUpdatedEvent)
	conn.NotifyBatchComplete(rdm, 7)

	req.Empty(testCh.sent)
	conn.svcSubscription.Lock()
	defer conn.svcSubscription.Unlock()
	req.Empty(conn.svcSubscription.pendingChecks)
}
