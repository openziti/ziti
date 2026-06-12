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

package sync_strats

import (
	"testing"

	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/stretchr/testify/require"
)

type stubTimelineSource string

func (s stubTimelineSource) TimelineId() string { return string(s) }

const (
	testRouterTargetTypeId  = "router-target.test"
	testServiceTargetTypeId = "service-target.test"
)

func newTestRtx(routerId string, routerConfigs []string, rdmConfigs map[string]*edge_ctrl_pb.DataState_Config) *RouterSender {
	rdm := common.NewRouterDataModelSender(stubTimelineSource("test-timeline"), 16, 1)
	rdm.ConfigTypes.Set(testRouterTargetTypeId, &edge_ctrl_pb.DataState_ConfigType{
		Id:     testRouterTargetTypeId,
		Name:   testRouterTargetTypeId,
		Target: common.ConfigTypeTargetRouter,
	})
	rdm.ConfigTypes.Set(testServiceTargetTypeId, &edge_ctrl_pb.DataState_ConfigType{
		Id:     testServiceTargetTypeId,
		Name:   testServiceTargetTypeId,
		Target: "service",
	})
	for id, cfg := range rdmConfigs {
		// default un-typed test configs to the router-target type, so existing
		// tests still exercise the filter path
		if cfg.TypeId == "" {
			cfg.TypeId = testRouterTargetTypeId
		}
		rdm.Configs.Set(id, cfg)
	}
	rdm.Routers.Set(routerId, &edge_ctrl_pb.DataState_Router{
		Id:      routerId,
		Configs: routerConfigs,
	})
	return &RouterSender{
		Router: &model.Router{
			BaseEntity: models.BaseEntity{Id: routerId},
			Configs:    routerConfigs,
		},
		routerDataModel: rdm,
	}
}

func cfgEvent(id string) *edge_ctrl_pb.DataState_Event {
	return &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_Config{
			Config: &edge_ctrl_pb.DataState_Config{Id: id, Name: id, TypeId: testRouterTargetTypeId},
		},
	}
}

func serviceCfgEvent(id string) *edge_ctrl_pb.DataState_Event {
	return &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_Config{
			Config: &edge_ctrl_pb.DataState_Config{Id: id, Name: id, TypeId: testServiceTargetTypeId},
		},
	}
}

func routerEvent(id string, configs []string, action edge_ctrl_pb.DataState_Action) *edge_ctrl_pb.DataState_Event {
	return &edge_ctrl_pb.DataState_Event{
		Action: action,
		Model: &edge_ctrl_pb.DataState_Event_Router{
			Router: &edge_ctrl_pb.DataState_Router{Id: id, Configs: configs},
		},
	}
}

func serviceEvent(id string) *edge_ctrl_pb.DataState_Event {
	return &edge_ctrl_pb.DataState_Event{
		Action: edge_ctrl_pb.DataState_Create,
		Model: &edge_ctrl_pb.DataState_Event_Service{
			Service: &edge_ctrl_pb.DataState_Service{Id: id},
		},
	}
}

func Test_filterEventsForRouter_dropsUnrelatedConfigs(t *testing.T) {
	req := require.New(t)
	rtx := newTestRtx("r1", []string{"cfg-A"}, map[string]*edge_ctrl_pb.DataState_Config{
		"cfg-A": {Id: "cfg-A"},
		"cfg-B": {Id: "cfg-B"},
	})

	in := []*edge_ctrl_pb.DataState_Event{
		cfgEvent("cfg-A"),
		cfgEvent("cfg-B"),
		serviceEvent("svc-1"),
	}
	out, changed := rtx.filterEventsForRouter(in, true)

	req.True(changed)
	req.Len(out, 2)
	req.Equal("cfg-A", out[0].Model.(*edge_ctrl_pb.DataState_Event_Config).Config.Id)
	req.Equal("svc-1", out[1].Model.(*edge_ctrl_pb.DataState_Event_Service).Service.Id)
}

func Test_filterEventsForRouter_passesNonConfigEvents(t *testing.T) {
	req := require.New(t)
	rtx := newTestRtx("r1", nil, nil)

	in := []*edge_ctrl_pb.DataState_Event{
		serviceEvent("svc-1"),
		{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_Identity{
				Identity: &edge_ctrl_pb.DataState_Identity{Id: "i1"},
			},
		},
	}
	out, changed := rtx.filterEventsForRouter(in, true)

	req.False(changed, "expected pass-through to leave events untouched")
	req.Same(&in[0], &out[0], "expected the same backing slice when no changes")
	req.Len(out, 2)
}

func Test_filterEventsForRouter_unchangedWhenAllConfigsBelongToRouter(t *testing.T) {
	req := require.New(t)
	rtx := newTestRtx("r1", []string{"cfg-A", "cfg-B"}, map[string]*edge_ctrl_pb.DataState_Config{
		"cfg-A": {Id: "cfg-A"},
		"cfg-B": {Id: "cfg-B"},
	})

	in := []*edge_ctrl_pb.DataState_Event{
		cfgEvent("cfg-A"),
		cfgEvent("cfg-B"),
		serviceEvent("svc-1"),
	}
	out, changed := rtx.filterEventsForRouter(in, true)

	req.False(changed)
	req.Same(&in[0], &out[0])
	req.Len(out, 3)
}

func Test_filterEventsForRouter_synthesizesConfigsForSelfRouterUpdate(t *testing.T) {
	req := require.New(t)
	rtx := newTestRtx("r1", []string{"cfg-A", "cfg-B"}, map[string]*edge_ctrl_pb.DataState_Config{
		"cfg-A": {Id: "cfg-A", Name: "A"},
		"cfg-B": {Id: "cfg-B", Name: "B"},
	})

	out, changed := rtx.filterEventsForRouter([]*edge_ctrl_pb.DataState_Event{
		routerEvent("r1", []string{"cfg-A", "cfg-B"}, edge_ctrl_pb.DataState_Update),
	}, true)

	req.True(changed)
	// Two synthetic Config Create events, then the original Router event.
	req.Len(out, 3)

	cfgA, ok := out[0].Model.(*edge_ctrl_pb.DataState_Event_Config)
	req.True(ok)
	req.Equal("cfg-A", cfgA.Config.Id)
	req.True(out[0].IsSynthetic)

	cfgB, ok := out[1].Model.(*edge_ctrl_pb.DataState_Event_Config)
	req.True(ok)
	req.Equal("cfg-B", cfgB.Config.Id)
	req.True(out[1].IsSynthetic)

	_, ok = out[2].Model.(*edge_ctrl_pb.DataState_Event_Router)
	req.True(ok)
}

func Test_filterEventsForRouter_otherRouterEventDoesNotSynthesize(t *testing.T) {
	req := require.New(t)
	rtx := newTestRtx("r1", []string{"cfg-A"}, map[string]*edge_ctrl_pb.DataState_Config{
		"cfg-A": {Id: "cfg-A"},
		"cfg-X": {Id: "cfg-X"},
	})

	out, changed := rtx.filterEventsForRouter([]*edge_ctrl_pb.DataState_Event{
		routerEvent("r2", []string{"cfg-X"}, edge_ctrl_pb.DataState_Update),
	}, true)

	req.False(changed)
	req.Len(out, 1)
	_, ok := out[0].Model.(*edge_ctrl_pb.DataState_Event_Router)
	req.True(ok)
}

func Test_filterEventsForRouter_fullSyncDoesNotSynthesize(t *testing.T) {
	req := require.New(t)
	rtx := newTestRtx("r1", []string{"cfg-A"}, map[string]*edge_ctrl_pb.DataState_Config{
		"cfg-A": {Id: "cfg-A"},
	})

	out, changed := rtx.filterEventsForRouter([]*edge_ctrl_pb.DataState_Event{
		routerEvent("r1", []string{"cfg-A"}, edge_ctrl_pb.DataState_Update),
	}, false)

	req.False(changed)
	req.Len(out, 1)
	_, ok := out[0].Model.(*edge_ctrl_pb.DataState_Event_Router)
	req.True(ok)
}

func Test_filterEventsForRouter_selfRouterDeleteDoesNotSynthesize(t *testing.T) {
	req := require.New(t)
	rtx := newTestRtx("r1", []string{"cfg-A"}, map[string]*edge_ctrl_pb.DataState_Config{
		"cfg-A": {Id: "cfg-A"},
	})

	out, changed := rtx.filterEventsForRouter([]*edge_ctrl_pb.DataState_Event{
		routerEvent("r1", []string{"cfg-A"}, edge_ctrl_pb.DataState_Delete),
	}, true)

	req.False(changed)
	req.Len(out, 1)
	_, ok := out[0].Model.(*edge_ctrl_pb.DataState_Event_Router)
	req.True(ok)
}

func Test_filterEventsForRouter_passesServiceTargetConfigs(t *testing.T) {
	req := require.New(t)
	// Router R1 has no router-target configs at all. A service-target config
	// (any target other than "router") must still flow.
	rtx := newTestRtx("r1", nil, nil)
	in := []*edge_ctrl_pb.DataState_Event{
		serviceCfgEvent("svc-cfg-A"),
		serviceCfgEvent("svc-cfg-B"),
	}
	out, changed := rtx.filterEventsForRouter(in, true)

	req.False(changed, "service-target configs should pass through unchanged")
	req.Same(&in[0], &out[0])
	req.Len(out, 2)
}

func Test_filterEventsForRouter_filtersOnlyRouterTargetConfigs(t *testing.T) {
	req := require.New(t)
	// Router R1 has router-target cfg-A. Service-target cfg-S1 always flows;
	// router-target cfg-B is dropped because it isn't in R1's list.
	rtx := newTestRtx("r1", []string{"cfg-A"}, map[string]*edge_ctrl_pb.DataState_Config{
		"cfg-A": {Id: "cfg-A"},
		"cfg-B": {Id: "cfg-B"},
	})

	out, changed := rtx.filterEventsForRouter([]*edge_ctrl_pb.DataState_Event{
		cfgEvent("cfg-A"),
		cfgEvent("cfg-B"),
		serviceCfgEvent("svc-cfg-1"),
	}, true)

	req.True(changed)
	req.Len(out, 2)
	req.Equal("cfg-A", out[0].Model.(*edge_ctrl_pb.DataState_Event_Config).Config.Id)
	req.Equal("svc-cfg-1", out[1].Model.(*edge_ctrl_pb.DataState_Event_Config).Config.Id)
}

func Test_filterEventsForRouter_unknownTypePassesThrough(t *testing.T) {
	req := require.New(t)
	// Defensive: a Config event whose type can't be resolved from the RDM cache
	// must NOT be silently dropped. It flows through (treated as non-router-target).
	rtx := newTestRtx("r1", nil, nil)
	in := []*edge_ctrl_pb.DataState_Event{
		{
			Action: edge_ctrl_pb.DataState_Create,
			Model: &edge_ctrl_pb.DataState_Event_Config{
				Config: &edge_ctrl_pb.DataState_Config{Id: "cfg-X", TypeId: "unknown-type"},
			},
		},
	}
	out, changed := rtx.filterEventsForRouter(in, true)

	req.False(changed)
	req.Len(out, 1)
}

func Test_filterEventsForRouter_dropsAllConfigsWhenRouterNotInRdmCache(t *testing.T) {
	req := require.New(t)
	// Router not yet in RDM cache (e.g. brand-new connect, BuildRouters hasn't run).
	// All Config events should be filtered out; non-Config events still flow.
	rtx := newTestRtx("r1", nil, map[string]*edge_ctrl_pb.DataState_Config{
		"cfg-A": {Id: "cfg-A"},
	})
	rtx.routerDataModel.Routers.Remove("r1")

	out, changed := rtx.filterEventsForRouter([]*edge_ctrl_pb.DataState_Event{
		cfgEvent("cfg-A"),
		serviceEvent("svc-1"),
	}, true)

	req.True(changed)
	req.Len(out, 1)
	_, ok := out[0].Model.(*edge_ctrl_pb.DataState_Event_Service)
	req.True(ok)
}

func Test_filterEventsForRouter_skipsSynthesisForUnknownConfig(t *testing.T) {
	req := require.New(t)
	// router R has [cfg-A, cfg-missing], but cfg-missing isn't in the RDM cache yet.
	rtx := newTestRtx("r1", []string{"cfg-A", "cfg-missing"}, map[string]*edge_ctrl_pb.DataState_Config{
		"cfg-A": {Id: "cfg-A"},
	})

	out, changed := rtx.filterEventsForRouter([]*edge_ctrl_pb.DataState_Event{
		routerEvent("r1", []string{"cfg-A", "cfg-missing"}, edge_ctrl_pb.DataState_Update),
	}, true)

	req.True(changed)
	// Only the Config for cfg-A is synthesized; the original Router event still flows.
	req.Len(out, 2)
	cfgA, ok := out[0].Model.(*edge_ctrl_pb.DataState_Event_Config)
	req.True(ok)
	req.Equal("cfg-A", cfgA.Config.Id)
	_, ok = out[1].Model.(*edge_ctrl_pb.DataState_Event_Router)
	req.True(ok)
}
