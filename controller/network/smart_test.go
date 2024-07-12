package network

import (
	"github.com/openziti/ziti/controller/model"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/transport/v2/tcp"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/xt"
	"github.com/stretchr/testify/assert"
)

func TestSmartRerouteMinCostDelta(t *testing.T) {
	ctx := model.NewTestContext(t)
	defer ctx.Cleanup()

	config := newTestConfig(ctx)
	config.options.MinRouterCost = 10
	config.options.Smart.MinCostDelta = 15
	defer close(config.closeNotify)

	network, err := NewNetwork(config, ctx)
	assert.Nil(t, err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	assert.Nil(t, err)

	r0 := model.NewRouterForTest("r0", "", transportAddr, nil, 0, true)
	network.Router.MarkConnected(r0)

	r1 := model.NewRouterForTest("r1", "", transportAddr, nil, 15, false)
	network.Router.MarkConnected(r1)

	r2 := model.NewRouterForTest("r2", "", transportAddr, nil, 0, false)
	network.Router.MarkConnected(r2)

	r3 := model.NewRouterForTest("r3", "", transportAddr, nil, 0, true)
	network.Router.MarkConnected(r3)

	newPathTestLink(network, "l0", r0, r1)
	link1 := newPathTestLink(network, "l1", r0, r2)
	newPathTestLink(network, "l2", r1, r3)
	newPathTestLink(network, "l3", r2, r3)

	svc := &model.Service{
		BaseEntity:         models.BaseEntity{Id: "svc"},
		Name:               "svc",
		TerminatorStrategy: "smartrouting",
	}

	lc := logcontext.NewContext()

	svc.Terminators = []*model.Terminator{
		{
			BaseEntity: models.BaseEntity{Id: "t0"},
			Service:    "svc",
			Router:     "r3",
			Binding:    "transport",
			Address:    "tcp:localhost:1001",
			InstanceId: "",
			Precedence: xt.Precedences.Default,
		},
	}

	params := newCircuitParams(svc, r0)
	_, terminator, pathNodes, _, cerr := network.selectPath(params, svc, "", lc)
	assert.NoError(t, cerr)

	path, pathErr := network.CreatePathWithNodes(pathNodes)
	assert.NoError(t, pathErr)

	assert.Equal(t, 2, len(path.Links))
	assert.Equal(t, "l1", path.Links[0].Id)
	assert.Equal(t, "l3", path.Links[1].Id)

	assert.Equal(t, int64(32), path.Cost(network.options.MinRouterCost))

	circuit := &model.Circuit{
		Id:         uuid.NewString(),
		ServiceId:  svc.Id,
		Path:       path,
		Terminator: terminator,
		CreatedAt:  time.Now(),
	}
	network.Circuit.Add(circuit)

	// r0 - r1 - r3 = 10 + 1 + 10 + 1 + 10 = 32
	// r0 - r2 - r3 = 10 + 1 + 15 + 1 + 10 = 37

	candidates := network.getRerouteCandidates()
	assert.Equal(t, 0, len(candidates))

	// updated cost would be 33, still better
	link1.SetStaticCost(2)

	candidates = network.getRerouteCandidates()
	assert.Equal(t, 0, len(candidates))

	// updated cost would be 37, same, shouldn't change
	link1.SetStaticCost(6)

	candidates = network.getRerouteCandidates()
	assert.Equal(t, 0, len(candidates))

	// updated cost would be 51, under threshold, shouldn't change
	link1.SetStaticCost(20)

	candidates = network.getRerouteCandidates()
	assert.Equal(t, 0, len(candidates))

	// updated cost would be 52, hit threshold, should change
	link1.SetStaticCost(21)

	candidates = network.getRerouteCandidates()
	assert.Equal(t, 1, len(candidates))

	candidate := candidates[0]
	assert.Equal(t, 2, len(candidate.path.Links))
	assert.Equal(t, "l0", candidate.path.Links[0].Id)
	assert.Equal(t, "l2", candidate.path.Links[1].Id)
}
