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

package common

import (
	"testing"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

type recordingRouterSubscriber struct {
	changed []string
	removed []string
}

func (r *recordingRouterSubscriber) OnRouterChanged(router *edge_ctrl_pb.DataState_Router) {
	r.changed = append(r.changed, router.Id)
}

func (r *recordingRouterSubscriber) OnRouterRemoved(routerId string) {
	r.removed = append(r.removed, routerId)
}

func routerEvent(action edge_ctrl_pb.DataState_Action, id string) (*edge_ctrl_pb.DataState_Event, *edge_ctrl_pb.DataState_Event_Router) {
	return &edge_ctrl_pb.DataState_Event{Action: action},
		&edge_ctrl_pb.DataState_Event_Router{Router: &edge_ctrl_pb.DataState_Router{Id: id}}
}

func Test_RouterSubscriber_DeleteDispatchesRemoved(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	rec := &recordingRouterSubscriber{}
	rdm.SetRouterSubscriber(rec)

	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Create, "r2"))
	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Delete, "r2"))

	req.False(rdm.Routers.Has("r2"))
	req.Equal([]string{"r2"}, rec.changed)
	req.Equal([]string{"r2"}, rec.removed)
}

// Test_RouterSubscriber_RemovedForUnknownRouter documents that the subscriber is
// told about a delete even when it never saw the router, so implementations must
// tolerate unknown ids.
func Test_RouterSubscriber_RemovedForUnknownRouter(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	rec := &recordingRouterSubscriber{}
	rdm.SetRouterSubscriber(rec)

	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Delete, "never-seen"))

	req.Empty(rec.changed)
	req.Equal([]string{"never-seen"}, rec.removed)
}

func Test_RouterSubscriber_CreateAndUpdateDispatchChanged(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	rec := &recordingRouterSubscriber{}
	rdm.SetRouterSubscriber(rec)

	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Create, "r2"))
	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Update, "r2"))
	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Create, "r3"))

	req.Equal([]string{"r2", "r2", "r3"}, rec.changed)
	req.Empty(rec.removed)
}

// Test_RouterSubscriber_DispatchSeesPostEventState pins the ordering contract: a
// subscriber reading the model during a callback sees the cache already updated.
func Test_RouterSubscriber_DispatchSeesPostEventState(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	var presentOnChange, presentOnRemove bool
	rdm.SetRouterSubscriber(&funcRouterSubscriber{
		changed: func(router *edge_ctrl_pb.DataState_Router) {
			presentOnChange = rdm.Routers.Has(router.Id)
		},
		removed: func(routerId string) {
			presentOnRemove = rdm.Routers.Has(routerId)
		},
	})

	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Create, "r2"))
	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Delete, "r2"))

	req.True(presentOnChange, "router should already be in the cache when OnRouterChanged fires")
	req.False(presentOnRemove, "router should already be out of the cache when OnRouterRemoved fires")
}

func Test_RouterSubscriber_ClearedSubscriberStopsDispatch(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	rec := &recordingRouterSubscriber{}
	rdm.SetRouterSubscriber(rec)
	req.Equal(RouterEventSubscriber(rec), rdm.RouterSubscriber())

	rdm.SetRouterSubscriber(nil)
	req.Nil(rdm.RouterSubscriber())

	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Create, "r2"))
	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Delete, "r2"))

	req.Empty(rec.changed)
	req.Empty(rec.removed)
}

// Test_RouterSubscriber_NoSubscriberNoPanic covers the default state, since most
// RouterDataModel instances never register a router subscriber.
func Test_RouterSubscriber_NoSubscriberNoPanic(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Create, "r2"))
	rdm.HandleRouterEvent(routerEvent(edge_ctrl_pb.DataState_Delete, "r2"))

	req.False(rdm.Routers.Has("r2"))
}

type funcRouterSubscriber struct {
	changed func(router *edge_ctrl_pb.DataState_Router)
	removed func(routerId string)
}

func (f *funcRouterSubscriber) OnRouterChanged(router *edge_ctrl_pb.DataState_Router) {
	f.changed(router)
}

func (f *funcRouterSubscriber) OnRouterRemoved(routerId string) {
	f.removed(routerId)
}
