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

package state

import (
	"testing"

	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/router/xlink"
	"github.com/stretchr/testify/require"
)

// fakeRegistry embeds the interface so only the one method the subscriber uses has to be real; any
// other call would panic loudly rather than pass silently.
type fakeRegistry struct {
	xlink.Registry
	removed      []string
	storeIndexes []uint64
}

func (f *fakeRegistry) RemoveLinkDest(id string, raftIndex uint64) {
	f.removed = append(f.removed, id)
	f.storeIndexes = append(f.storeIndexes, raftIndex)
}

func Test_LinkDestSubscriber_RemovedForwardsToRegistry(t *testing.T) {
	req := require.New(t)
	reg := &fakeRegistry{}

	sub := LinkDestSubscriber{Registry: reg}
	sub.OnRouterRemoved("r1")
	sub.OnRouterRemoved("r2")

	req.Equal([]string{"r1", "r2"}, reg.removed)
	req.Equal([]uint64{0, 0}, reg.storeIndexes,
		"the data model sequences its own changes, not the one peer state is ordered by")
}

func Test_LinkDestSubscriber_ChangedIsNoOp(t *testing.T) {
	req := require.New(t)
	reg := &fakeRegistry{}

	sub := LinkDestSubscriber{Registry: reg}
	sub.OnRouterChanged(&edge_ctrl_pb.DataState_Router{Id: "r1"})

	req.Empty(reg.removed)
}

// Test_LinkDestSubscriber_DeleteEventReachesRegistry walks the fast path end to
// end: a delete arriving from the controller reaches the registry.
func Test_LinkDestSubscriber_DeleteEventReachesRegistry(t *testing.T) {
	req := require.New(t)
	reg := &fakeRegistry{}

	rdm := common.NewBareRouterDataModel("self")
	rdm.SetRouterSubscriber(LinkDestSubscriber{Registry: reg})

	rdm.HandleRouterEvent(
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Delete},
		&edge_ctrl_pb.DataState_Event_Router{Router: &edge_ctrl_pb.DataState_Router{Id: "gone"}},
	)

	req.Equal([]string{"gone"}, reg.removed)
}

// Test_LinkDestSubscriber_FullStateGapReachesRegistry walks the path the fast
// path cannot cover: the router was disconnected when "gone" was deleted, so the
// delete is only visible as its absence from the replacement model.
func Test_LinkDestSubscriber_FullStateGapReachesRegistry(t *testing.T) {
	req := require.New(t)
	closeNotify := make(chan struct{})
	defer close(closeNotify)

	reg := &fakeRegistry{}
	mgr := &ManagerImpl{}

	// Both models carry the local router's own record, as a source that distributes routers does.
	existing := common.NewReceiverRouterDataModel("self", closeNotify)
	existing.Routers.Set("self", &edge_ctrl_pb.DataState_Router{Id: "self"})
	existing.Routers.Set("keep", &edge_ctrl_pb.DataState_Router{Id: "keep"})
	existing.Routers.Set("gone", &edge_ctrl_pb.DataState_Router{Id: "gone"})
	mgr.SetRouterDataModel(existing, false)
	mgr.SetRouterEventSubscriber(LinkDestSubscriber{Registry: reg})

	updated := common.NewReceiverRouterDataModel("self", closeNotify)
	updated.Routers.Set("self", &edge_ctrl_pb.DataState_Router{Id: "self"})
	updated.Routers.Set("keep", &edge_ctrl_pb.DataState_Router{Id: "keep"})
	mgr.SetRouterDataModel(updated, false)

	req.Equal([]string{"gone"}, reg.removed)
}

// Test_LinkDestSubscriber_OlderControllerDoesNotClearDestinations: a controller from before routers were
// distributed in the data model sends a full state carrying none, and the router keeps its destinations
// rather than reading that as every router in the fleet having been deleted.
func Test_LinkDestSubscriber_OlderControllerDoesNotClearDestinations(t *testing.T) {
	req := require.New(t)
	closeNotify := make(chan struct{})
	defer close(closeNotify)

	reg := &fakeRegistry{}
	mgr := &ManagerImpl{}

	existing := common.NewReceiverRouterDataModel("self", closeNotify)
	existing.Routers.Set("self", &edge_ctrl_pb.DataState_Router{Id: "self"})
	existing.Routers.Set("peer1", &edge_ctrl_pb.DataState_Router{Id: "peer1"})
	existing.Routers.Set("peer2", &edge_ctrl_pb.DataState_Router{Id: "peer2"})
	mgr.SetRouterDataModel(existing, false)
	mgr.SetRouterEventSubscriber(LinkDestSubscriber{Registry: reg})

	// The subscription moves to a controller that does not distribute routers at all.
	mgr.SetRouterDataModel(common.NewReceiverRouterDataModel("self", closeNotify), false)

	req.Empty(reg.removed, "a full state with no routers must not tear down every link destination")
}
