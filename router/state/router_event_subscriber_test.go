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
	"github.com/stretchr/testify/require"
)

// recordingRouterSub captures router subscriber notifications. Map iteration is
// non-deterministic, so assertions use ElementsMatch rather than ordering.
type recordingRouterSub struct {
	changed []string
	removed []string
}

func (r *recordingRouterSub) OnRouterChanged(router *edge_ctrl_pb.DataState_Router) {
	r.changed = append(r.changed, router.Id)
}

func (r *recordingRouterSub) OnRouterRemoved(routerId string) {
	r.removed = append(r.removed, routerId)
}

// newRdmWithRouters builds a model holding the given routers, as a source that distributes routers sends
// it: such a source sends all of them, so the local router's own record is always there. The value of each
// entry becomes the router's Name, so a test can make two records differ.
func newRdmWithRouters(routers map[string]string) *common.RouterDataModel {
	rdm := newRdmWithoutSelf(routers)
	rdm.Routers.Set("self", &edge_ctrl_pb.DataState_Router{Id: "self"})
	return rdm
}

// newRdmWithoutSelf builds a model whose router set omits the local router, which is what a source that
// does not distribute routers produces: with no routers at all, or with whatever a stale set left behind.
func newRdmWithoutSelf(routers map[string]string) *common.RouterDataModel {
	rdm := common.NewBareRouterDataModel("self")
	for id, name := range routers {
		rdm.Routers.Set(id, &edge_ctrl_pb.DataState_Router{Id: id, Name: name})
	}
	return rdm
}

func Test_DispatchRouterDiff_BothNilNoOp(t *testing.T) {
	rec := &recordingRouterSub{}
	dispatchRouterDiff(nil, nil, rec)
	require.Empty(t, rec.changed)
	require.Empty(t, rec.removed)
}

func Test_DispatchRouterDiff_NilSubscriberNoPanic(t *testing.T) {
	dispatchRouterDiff(nil, newRdmWithRouters(map[string]string{"r1": "one"}), nil)
	// no panic = pass
}

func Test_DispatchRouterDiff_OnlyNewState_AllChanged(t *testing.T) {
	req := require.New(t)
	rec := &recordingRouterSub{}

	dispatchRouterDiff(nil, newRdmWithRouters(map[string]string{"r1": "one", "r2": "two"}), rec)

	req.Empty(rec.removed)
	req.ElementsMatch([]string{"r1", "r2", "self"}, rec.changed)
}

// Test_DispatchRouterDiff_AbsentFromNewStateIsRemoved is the case the diff exists for: a full state carries
// nothing at all for a deleted router, so its absence from the new model is the only evidence it is gone.
func Test_DispatchRouterDiff_AbsentFromNewStateIsRemoved(t *testing.T) {
	req := require.New(t)
	rec := &recordingRouterSub{}

	dispatchRouterDiff(newRdmWithRouters(map[string]string{"r1": "one", "r2": "two"}), newRdmWithRouters(nil), rec)

	req.Empty(rec.changed)
	req.ElementsMatch([]string{"r1", "r2"}, rec.removed)
}

// Test_DispatchRouterDiff_SourceWithoutRoutersRemovesNothing is the one that matters in a mixed-version
// cluster. A controller from before routers were distributed in the data model sends a full state with none
// of them, and reading that as a deletion of every router drops every link destination this router has.
func Test_DispatchRouterDiff_SourceWithoutRoutersRemovesNothing(t *testing.T) {
	req := require.New(t)
	rec := &recordingRouterSub{}

	oldRdm := newRdmWithRouters(map[string]string{"r1": "one", "r2": "two"})

	dispatchRouterDiff(oldRdm, newRdmWithoutSelf(nil), rec)

	req.Empty(rec.removed, "an empty router set is evidence of nothing, not of a fleet-wide deletion")
	req.Empty(rec.changed)
}

// Test_DispatchRouterDiff_NewStateWithoutSelfRemovesNothing: a source that distributes routers sends all of
// them, so one whose set omits this router is not a source to read deletions from, however many it carries.
func Test_DispatchRouterDiff_NewStateWithoutSelfRemovesNothing(t *testing.T) {
	req := require.New(t)
	rec := &recordingRouterSub{}

	oldRdm := newRdmWithRouters(map[string]string{"r1": "one", "gone": "bye"})

	dispatchRouterDiff(oldRdm, newRdmWithoutSelf(map[string]string{"r1": "one"}), rec)

	req.Empty(rec.removed)
}

// Test_DispatchRouterDiff_NilNewStateRemovesNothing: there is no model to have carried routers, so there is
// nothing to conclude from what it does not hold.
func Test_DispatchRouterDiff_NilNewStateRemovesNothing(t *testing.T) {
	req := require.New(t)
	rec := &recordingRouterSub{}

	dispatchRouterDiff(newRdmWithRouters(map[string]string{"r1": "one"}), nil, rec)

	req.Empty(rec.removed)
	req.Empty(rec.changed)
}

func Test_DispatchRouterDiff_MixedAddRemoveKeep(t *testing.T) {
	req := require.New(t)
	rec := &recordingRouterSub{}

	oldRdm := newRdmWithRouters(map[string]string{"keep": "same", "gone": "bye"})
	newRdm := newRdmWithRouters(map[string]string{"keep": "same", "added": "hi"})

	dispatchRouterDiff(oldRdm, newRdm, rec)

	req.Equal([]string{"gone"}, rec.removed)
	req.Equal([]string{"added"}, rec.changed, "unchanged router should not be redispatched")
}

func Test_DispatchRouterDiff_UnchangedRoutersSkipped(t *testing.T) {
	req := require.New(t)
	rec := &recordingRouterSub{}

	routers := map[string]string{"r1": "one", "r2": "two"}
	dispatchRouterDiff(newRdmWithRouters(routers), newRdmWithRouters(routers), rec)

	req.Empty(rec.changed)
	req.Empty(rec.removed)
}

func Test_DispatchRouterDiff_ChangedRecordRedispatched(t *testing.T) {
	req := require.New(t)
	rec := &recordingRouterSub{}

	oldRdm := newRdmWithRouters(map[string]string{"r1": "one"})
	newRdm := newRdmWithRouters(map[string]string{"r1": "renamed"})

	dispatchRouterDiff(oldRdm, newRdm, rec)

	req.Empty(rec.removed)
	req.Equal([]string{"r1"}, rec.changed)
}

func Test_CollectRouters_NilRdm(t *testing.T) {
	require.Empty(t, collectRouters(nil))
}

// Test_SetRouterDataModel_DispatchesRouterDiff covers the wiring, not the diff:
// that the manager bootstraps a newly attached subscriber, re-attaches it to the
// replacement model, and runs the diff across the swap.
func Test_SetRouterDataModel_DispatchesRouterDiff(t *testing.T) {
	req := require.New(t)
	closeNotify := make(chan struct{})
	defer close(closeNotify)

	mgr := &ManagerImpl{}
	rec := &recordingRouterSub{}

	// Both models carry the local router's own record, as a source that distributes routers does.
	existing := common.NewReceiverRouterDataModel("self", closeNotify)
	existing.Routers.Set("self", &edge_ctrl_pb.DataState_Router{Id: "self"})
	existing.Routers.Set("keep", &edge_ctrl_pb.DataState_Router{Id: "keep"})
	existing.Routers.Set("gone", &edge_ctrl_pb.DataState_Router{Id: "gone"})
	mgr.SetRouterDataModel(existing, false)

	mgr.SetRouterEventSubscriber(rec)
	req.ElementsMatch([]string{"self", "keep", "gone"}, rec.changed, "attach should bootstrap against the current model")
	req.Empty(rec.removed)
	rec.changed = nil

	updated := common.NewReceiverRouterDataModel("self", closeNotify)
	updated.Routers.Set("self", &edge_ctrl_pb.DataState_Router{Id: "self"})
	updated.Routers.Set("keep", &edge_ctrl_pb.DataState_Router{Id: "keep"})
	updated.Routers.Set("added", &edge_ctrl_pb.DataState_Router{Id: "added"})
	mgr.SetRouterDataModel(updated, false)

	req.Equal([]string{"gone"}, rec.removed, "a router missing from the new model is a delete")
	req.Equal([]string{"added"}, rec.changed)

	// The subscriber must also be live on the replacement model, not just diffed across it.
	rec.removed = nil
	updated.HandleRouterEvent(
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Delete},
		&edge_ctrl_pb.DataState_Event_Router{Router: &edge_ctrl_pb.DataState_Router{Id: "keep"}},
	)
	req.Equal([]string{"keep"}, rec.removed)
}
