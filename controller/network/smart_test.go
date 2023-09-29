package network

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/common/logcontext"
	"github.com/openziti/transport/v2/tcp"
	"github.com/stretchr/testify/assert"
)

func TestSmartRerouteMinCostDelta(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	config := newTestConfig(ctx)
	config.options.MinRouterCost = 10
	config.options.Smart.MinCostDelta = 15
	defer close(config.closeNotify)

	network, err := NewNetwork(config)
	assert.Nil(t, err)

	addr := "tcp:0.0.0.0:0"
	transportAddr, err := tcp.AddressParser{}.Parse(addr)
	assert.Nil(t, err)

	r0 := newRouterForTest("r0", "", transportAddr, nil, 0, true)
	network.Routers.markConnected(r0)

	r1 := newRouterForTest("r1", "", transportAddr, nil, 15, false)
	network.Routers.markConnected(r1)

	r2 := newRouterForTest("r2", "", transportAddr, nil, 0, false)
	network.Routers.markConnected(r2)

	r3 := newRouterForTest("r3", "", transportAddr, nil, 0, true)
	network.Routers.markConnected(r3)

	newPathTestLink(network, "l0", r0, r1)
	link1 := newPathTestLink(network, "l1", r0, r2)
	newPathTestLink(network, "l2", r1, r3)
	newPathTestLink(network, "l3", r2, r3)

	svc := &Service{
		BaseEntity:         models.BaseEntity{Id: "svc"},
		Name:               "svc",
		TerminatorStrategy: "smartrouting",
	}

	lc := logcontext.NewContext()

	svc.Terminators = []*Terminator{
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

	_, terminator, pathNodes, cerr := network.selectPath(r0, svc, "", lc)
	assert.NoError(t, cerr)

	path, pathErr := network.CreatePathWithNodes(pathNodes)
	assert.NoError(t, pathErr)

	assert.Equal(t, 2, len(path.Links))
	assert.Equal(t, "l1", path.Links[0].Id)
	assert.Equal(t, "l3", path.Links[1].Id)

	assert.Equal(t, int64(32), path.cost(network.options.MinRouterCost))

	circuit := &Circuit{
		Id:         uuid.NewString(),
		Service:    svc,
		Path:       path,
		Terminator: terminator,
		CreatedAt:  time.Now(),
	}
	network.circuitController.add(circuit)

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
