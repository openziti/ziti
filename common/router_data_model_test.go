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

func seedConfigType(rdm *RouterDataModel, id, target string) {
	rdm.ConfigTypes.Set(id, &ConfigType{
		Id:     id,
		Name:   id,
		Target: target,
	})
}

func seedConfig(rdm *RouterDataModel, id, typeId string) {
	rdm.Configs.Set(id, &Config{
		Id:     id,
		Name:   id,
		TypeId: typeId,
	})
}

func Test_HandleRouterEvent_GCsRouterTargetOrphansOnSelf(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	seedConfigType(rdm, "router-type", "router")
	seedConfigType(rdm, "service-type", "service")
	seedConfig(rdm, "rcfg-A", "router-type")
	seedConfig(rdm, "rcfg-B", "router-type")
	seedConfig(rdm, "rcfg-C", "router-type")
	seedConfig(rdm, "scfg-1", "service-type")

	// Initial association: r1 has rcfg-A and rcfg-B.
	rdm.Routers.Set("r1", &edge_ctrl_pb.DataState_Router{
		Id:      "r1",
		Configs: []string{"rcfg-A", "rcfg-B"},
	})

	// Update r1 to drop rcfg-B and add rcfg-C.
	updated := &edge_ctrl_pb.DataState_Router{
		Id:      "r1",
		Configs: []string{"rcfg-A", "rcfg-C"},
	}
	rdm.HandleRouterEvent(
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Update},
		&edge_ctrl_pb.DataState_Event_Router{Router: updated},
	)

	req.Equal(1, rdm.Routers.Count())
	// rcfg-B got GC'd; rcfg-A and rcfg-C remain.
	req.True(rdm.Configs.Has("rcfg-A"))
	req.False(rdm.Configs.Has("rcfg-B"), "stale router-target config should be GC'd")
	req.True(rdm.Configs.Has("rcfg-C"))
	// service-target config untouched.
	req.True(rdm.Configs.Has("scfg-1"))
}

// Test_HandleRouterEvent_GcDispatchesSubscriberRemove ensures the
// router-event-driven orphan GC dispatches OnRouterConfigRemoved through
// the subscriber. Without this, removing a Config from a router's
// Configs list would silently leave the router-side listener bound.
func Test_HandleRouterEvent_GcDispatchesSubscriberRemove(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	seedConfigType(rdm, "router-link", ConfigTypeTargetRouter)
	rdm.ConfigTypes.Set("router-link", &ConfigType{Id: "router-link", Name: "router.link.v1", Target: ConfigTypeTargetRouter})
	seedConfig(rdm, "rcfg-A", "router-link")

	rec := &recordingRouterConfigSubscriber{}
	rdm.SetRouterConfigSubscriber(rec)

	rdm.Routers.Set("r1", &edge_ctrl_pb.DataState_Router{
		Id:      "r1",
		Configs: []string{"rcfg-A"},
	})

	// Drop the config from the router's Configs list.
	rdm.HandleRouterEvent(
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Update},
		&edge_ctrl_pb.DataState_Event_Router{Router: &edge_ctrl_pb.DataState_Router{
			Id:      "r1",
			Configs: []string{},
		}},
	)

	req.False(rdm.Configs.Has("rcfg-A"), "orphaned config should be GC'd")
	req.Equal([]string{"router.link.v1"}, rec.removed, "GC must dispatch a remove event to the subscriber")
}

func Test_HandleRouterEvent_NoGCForOtherRouter(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	seedConfigType(rdm, "router-type", "router")
	seedConfig(rdm, "rcfg-A", "router-type")
	seedConfig(rdm, "rcfg-B", "router-type")

	// r1 still associated with both.
	rdm.Routers.Set("r1", &edge_ctrl_pb.DataState_Router{
		Id:      "r1",
		Configs: []string{"rcfg-A", "rcfg-B"},
	})

	// A different router updates with no configs. Must NOT GC anything in the
	// receiver's view.
	rdm.HandleRouterEvent(
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Update},
		&edge_ctrl_pb.DataState_Event_Router{Router: &edge_ctrl_pb.DataState_Router{
			Id:      "r2",
			Configs: nil,
		}},
	)

	req.True(rdm.Configs.Has("rcfg-A"))
	req.True(rdm.Configs.Has("rcfg-B"))
}

func Test_HandleRouterEvent_NoGCWhenSelfNotSet(t *testing.T) {
	req := require.New(t)
	// Receiver-side RDM with no self ID (e.g. controller-side construction
	// or validate-snapshot parsing). HandleRouterEvent must not GC anything.
	rdm := NewBareRouterDataModel("")

	seedConfigType(rdm, "router-type", "router")
	seedConfig(rdm, "rcfg-A", "router-type")

	rdm.HandleRouterEvent(
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Update},
		&edge_ctrl_pb.DataState_Event_Router{Router: &edge_ctrl_pb.DataState_Router{
			Id:      "r1",
			Configs: nil,
		}},
	)

	req.True(rdm.Configs.Has("rcfg-A"), "should not GC when selfRouterId is unset")
}

// Test_HandleRouterEvent_FirstSeenNoGC covers the case where this is the
// first Router event we've seen for the local router. There's no
// previously-cached Configs list to diff against, so nothing to GC; the
// sender's synthesized Config events that came with the Router event are
// what populate the cache.
func Test_HandleRouterEvent_FirstSeenNoGC(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	seedConfigType(rdm, "router-type", "router")
	seedConfig(rdm, "rcfg-A", "router-type")

	// No prior rdm.Routers entry for r1.
	rdm.HandleRouterEvent(
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Update},
		&edge_ctrl_pb.DataState_Event_Router{Router: &edge_ctrl_pb.DataState_Router{
			Id:      "r1",
			Configs: []string{"rcfg-A"},
		}},
	)

	req.True(rdm.Configs.Has("rcfg-A"), "newly-assigned config should remain")
	req.Equal(1, rdm.Routers.Count())
}

func Test_HandleRouterEvent_GCEmptyConfigs(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	seedConfigType(rdm, "router-type", "router")
	seedConfigType(rdm, "service-type", "service")
	seedConfig(rdm, "rcfg-A", "router-type")
	seedConfig(rdm, "rcfg-B", "router-type")
	seedConfig(rdm, "scfg-1", "service-type")

	rdm.Routers.Set("r1", &edge_ctrl_pb.DataState_Router{
		Id:      "r1",
		Configs: []string{"rcfg-A", "rcfg-B"},
	})

	// Disassociate everything.
	rdm.HandleRouterEvent(
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Update},
		&edge_ctrl_pb.DataState_Event_Router{Router: &edge_ctrl_pb.DataState_Router{
			Id:      "r1",
			Configs: nil,
		}},
	)

	req.False(rdm.Configs.Has("rcfg-A"))
	req.False(rdm.Configs.Has("rcfg-B"))
	req.True(rdm.Configs.Has("scfg-1"), "service configs untouched")
}

func Test_HandleRouterEvent_DeleteSelf(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")

	seedConfigType(rdm, "router-type", "router")
	seedConfig(rdm, "rcfg-A", "router-type")

	rdm.Routers.Set("r1", &edge_ctrl_pb.DataState_Router{
		Id:      "r1",
		Configs: []string{"rcfg-A"},
	})

	// Delete event for self: only Router entity is removed; configs stay (they
	// would be cleaned up via subsequent ConfigDelete events).
	rdm.HandleRouterEvent(
		&edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Delete},
		&edge_ctrl_pb.DataState_Event_Router{Router: &edge_ctrl_pb.DataState_Router{Id: "r1"}},
	)

	req.False(rdm.Routers.Has("r1"))
	req.True(rdm.Configs.Has("rcfg-A"))
}

// --- Router config subscriber dispatch ---------------------------------------

type recordingRouterConfigSubscriber struct {
	applied []appliedEntry
	removed []string
}

type appliedEntry struct {
	configType string
	data       string
}

func (r *recordingRouterConfigSubscriber) OnRouterConfigApplied(configType string, data string) {
	r.applied = append(r.applied, appliedEntry{configType: configType, data: data})
}

func (r *recordingRouterConfigSubscriber) OnRouterConfigRemoved(configType string) {
	r.removed = append(r.removed, configType)
}

// configEvent builds a Config Create/Update event suitable for HandleConfigEvent.
func configEvent(action edge_ctrl_pb.DataState_Action, configId, typeId, dataJson string) (*edge_ctrl_pb.DataState_Event, *edge_ctrl_pb.DataState_Event_Config) {
	model := &edge_ctrl_pb.DataState_Event_Config{
		Config: &edge_ctrl_pb.DataState_Config{
			Id:       configId,
			Name:     configId,
			TypeId:   typeId,
			DataJson: dataJson,
		},
	}
	return &edge_ctrl_pb.DataState_Event{Action: action, Model: model}, model
}

func Test_HandleConfigEvent_DispatchesRouterTargetApply(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")
	rdm.ConfigTypes.Set("router-link", &ConfigType{Id: "router-link", Name: "router.link.v1", Target: ConfigTypeTargetRouter})
	rec := &recordingRouterConfigSubscriber{}
	rdm.SetRouterConfigSubscriber(rec)

	event, model := configEvent(edge_ctrl_pb.DataState_Create, "cfg-A", "router-link", `{"k":"v"}`)
	rdm.HandleConfigEvent(1, event, model)

	req.Len(rec.applied, 1)
	req.Equal("router.link.v1", rec.applied[0].configType)
	req.Equal(`{"k":"v"}`, rec.applied[0].data)
	req.Empty(rec.removed)
}

func Test_HandleConfigEvent_DispatchesRouterTargetRemove(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")
	rdm.ConfigTypes.Set("router-link", &ConfigType{Id: "router-link", Name: "router.link.v1", Target: ConfigTypeTargetRouter})
	seedConfig(rdm, "cfg-A", "router-link")
	rec := &recordingRouterConfigSubscriber{}
	rdm.SetRouterConfigSubscriber(rec)

	event, model := configEvent(edge_ctrl_pb.DataState_Delete, "cfg-A", "router-link", "")
	rdm.HandleConfigEvent(1, event, model)

	req.Empty(rec.applied)
	req.Equal([]string{"router.link.v1"}, rec.removed)
}

func Test_HandleConfigEvent_NonRouterTargetNotDispatched(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")
	rdm.ConfigTypes.Set("service-cfg", &ConfigType{Id: "service-cfg", Name: "service.cfg.v1", Target: "service"})
	rec := &recordingRouterConfigSubscriber{}
	rdm.SetRouterConfigSubscriber(rec)

	event, model := configEvent(edge_ctrl_pb.DataState_Create, "cfg-X", "service-cfg", `{}`)
	rdm.HandleConfigEvent(1, event, model)

	req.Empty(rec.applied)
	req.Empty(rec.removed)
}

func Test_HandleConfigEvent_NoSubscriberNoPanic(t *testing.T) {
	rdm := NewBareRouterDataModel("r1")
	rdm.ConfigTypes.Set("router-link", &ConfigType{Id: "router-link", Name: "router.link.v1", Target: ConfigTypeTargetRouter})

	event, model := configEvent(edge_ctrl_pb.DataState_Create, "cfg-A", "router-link", `{}`)
	// Must not panic with no subscriber set.
	rdm.HandleConfigEvent(1, event, model)

	delEvent, delModel := configEvent(edge_ctrl_pb.DataState_Delete, "cfg-A", "router-link", "")
	rdm.HandleConfigEvent(2, delEvent, delModel)
}

func Test_HandleConfigEvent_UnknownConfigTypeNotDispatched(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")
	// No ConfigType registered: dispatcher must skip silently.
	rec := &recordingRouterConfigSubscriber{}
	rdm.SetRouterConfigSubscriber(rec)

	event, model := configEvent(edge_ctrl_pb.DataState_Create, "cfg-A", "missing-type", `{}`)
	rdm.HandleConfigEvent(1, event, model)

	req.Empty(rec.applied)
	req.Empty(rec.removed)
}

func Test_HandleConfigEvent_NoChangeNotDispatched(t *testing.T) {
	req := require.New(t)
	rdm := NewBareRouterDataModel("r1")
	rdm.ConfigTypes.Set("router-link", &ConfigType{Id: "router-link", Name: "router.link.v1", Target: ConfigTypeTargetRouter})
	rec := &recordingRouterConfigSubscriber{}
	rdm.SetRouterConfigSubscriber(rec)

	event, model := configEvent(edge_ctrl_pb.DataState_Create, "cfg-A", "router-link", `{"k":"v"}`)
	rdm.HandleConfigEvent(1, event, model)
	req.Len(rec.applied, 1, "first event should dispatch")

	// Identical second event must not redispatch.
	rdm.HandleConfigEvent(2, event, model)
	req.Len(rec.applied, 1, "identical-data event should not redispatch")

	// Update with different data dispatches.
	changedEvent, changedModel := configEvent(edge_ctrl_pb.DataState_Update, "cfg-A", "router-link", `{"k":"v2"}`)
	rdm.HandleConfigEvent(3, changedEvent, changedModel)
	req.Len(rec.applied, 2, "changed-data event should dispatch")
	req.Equal(`{"k":"v2"}`, rec.applied[1].data)
}
