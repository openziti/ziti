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
