package xt_weighted

import (
	"math"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/controller/xt"
	"github.com/stretchr/testify/require"
)

type mockTerminator struct {
	id         string
	precedence xt.Precedence
	routeCost  uint32
}

func (m *mockTerminator) GetId() string                { return m.id }
func (m *mockTerminator) GetPrecedence() xt.Precedence { return m.precedence }
func (m *mockTerminator) GetRouteCost() uint32         { return m.routeCost }
func (m *mockTerminator) GetCost() uint16              { return 0 }
func (m *mockTerminator) GetServiceId() string         { return "svc" }
func (m *mockTerminator) GetInstanceId() string        { return "" }
func (m *mockTerminator) GetRouterId() string          { return "r1" }
func (m *mockTerminator) GetBinding() string           { return "" }
func (m *mockTerminator) GetAddress() string           { return "" }
func (m *mockTerminator) GetPeerData() xt.PeerData     { return nil }
func (m *mockTerminator) GetCreatedAt() time.Time      { return time.Time{} }
func (m *mockTerminator) GetHostId() string            { return "" }
func (m *mockTerminator) GetSourceCtrl() string        { return "" }

func newTerminator(id string, cost uint32) xt.CostedTerminator {
	return &mockTerminator{
		id:         id,
		precedence: xt.Precedences.Default,
		routeCost:  xt.Precedences.Default.GetBiasedCost(cost),
	}
}

func TestWeightedStrategy_AllTerminatorsSelected(t *testing.T) {
	req := require.New(t)

	s := &strategy{}

	terminators := []xt.CostedTerminator{
		newTerminator("a", 10),
		newTerminator("b", 10),
		newTerminator("c", 10),
	}

	counts := map[string]int{}
	iterations := 30000

	for range iterations {
		selected, _, err := s.Select(nil, terminators)
		req.NoError(err)
		counts[selected.GetId()]++
	}

	// With equal costs, each should be selected roughly 1/3 of the time.
	// Use a generous tolerance of 15% to avoid flaky tests.
	expectedPer := float64(iterations) / 3.0
	tolerance := 0.15

	for _, id := range []string{"a", "b", "c"} {
		count := counts[id]
		req.Greater(count, 0, "terminator %s was never selected", id)
		deviation := math.Abs(float64(count)-expectedPer) / expectedPer
		req.Less(deviation, tolerance,
			"terminator %s selected %d times (expected ~%d, deviation %.1f%%)",
			id, count, int(expectedPer), deviation*100)
	}
}

func TestWeightedStrategy_LowerCostPreferred(t *testing.T) {
	req := require.New(t)

	s := &strategy{}

	terminators := []xt.CostedTerminator{
		newTerminator("cheap", 1),
		newTerminator("expensive", 100),
	}

	counts := map[string]int{}
	iterations := 10000

	for range iterations {
		selected, _, err := s.Select(nil, terminators)
		req.NoError(err)
		counts[selected.GetId()]++
	}

	// The cheaper terminator should be selected significantly more often
	req.Greater(counts["cheap"], counts["expensive"],
		"cheaper terminator should be selected more often: cheap=%d, expensive=%d",
		counts["cheap"], counts["expensive"])
}

func TestWeightedStrategy_FiveTerminatorsAllSelected(t *testing.T) {
	req := require.New(t)

	s := &strategy{}

	terminators := []xt.CostedTerminator{
		newTerminator("a", 10),
		newTerminator("b", 20),
		newTerminator("c", 30),
		newTerminator("d", 40),
		newTerminator("e", 50),
	}

	counts := map[string]int{}
	iterations := 50000

	for range iterations {
		selected, _, err := s.Select(nil, terminators)
		req.NoError(err)
		counts[selected.GetId()]++
	}

	// Every terminator must be selected at least once
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		req.Greater(counts[id], 0, "terminator %s was never selected", id)
	}

	// Lower cost terminators should be selected more often
	req.Greater(counts["a"], counts["e"],
		"lowest cost terminator should be selected more than highest cost: a=%d, e=%d",
		counts["a"], counts["e"])
}
