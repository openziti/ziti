package xt

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrecedenceRangesDoNotOverlap(t *testing.T) {
	precedences := []Precedence{
		Precedences.Required,
		Precedences.Default,
		Precedences.Failed,
		Precedences.unknown,
	}

	for _, p := range precedences {
		pp := p.(*precedence)
		require.LessOrEqual(t, pp.minCost, pp.maxCost, "precedence %s has minCost > maxCost", pp.name)
	}

	for i := 0; i < len(precedences)-1; i++ {
		curr := precedences[i].(*precedence)
		next := precedences[i+1].(*precedence)
		require.Less(t, curr.maxCost, next.minCost,
			"precedence %s [%d, %d] overlaps with %s [%d, %d]",
			curr.name, curr.minCost, curr.maxCost,
			next.name, next.minCost, next.maxCost)
	}
}

func TestPrecedenceRangesCoverFullUint32Space(t *testing.T) {
	require.Equal(t, uint32(0), Precedences.Required.GetBaseCost(), "Required should start at 0")
	require.Equal(t, uint32(math.MaxUint32), Precedences.unknown.(*precedence).maxCost, "unknown should end at MaxUint32")

	// verify adjacent ranges are contiguous (no gaps)
	pairs := []struct{ lower, higher Precedence }{
		{Precedences.Required, Precedences.Default},
		{Precedences.Default, Precedences.Failed},
		{Precedences.Failed, Precedences.unknown},
	}
	for _, pair := range pairs {
		lower := pair.lower.(*precedence)
		higher := pair.higher.(*precedence)
		require.Equal(t, lower.maxCost+1, higher.minCost,
			"gap between %s and %s", lower.name, higher.name)
	}
}

func TestGetBiasedCostClampsWithoutOverflow(t *testing.T) {
	precedences := []Precedence{
		Precedences.Required,
		Precedences.Default,
		Precedences.Failed,
		Precedences.unknown,
	}

	for _, p := range precedences {
		pp := p.(*precedence)
		t.Run(pp.name, func(t *testing.T) {
			// zero cost returns minCost
			require.Equal(t, pp.minCost, pp.GetBiasedCost(0))

			// cost that fits returns minCost + cost
			require.Equal(t, pp.minCost+1, pp.GetBiasedCost(1))

			// cost at range limit returns maxCost
			rangeWidth := pp.maxCost - pp.minCost
			require.Equal(t, pp.maxCost, pp.GetBiasedCost(rangeWidth))

			// cost exceeding range clamps to maxCost (not overflow)
			require.Equal(t, pp.maxCost, pp.GetBiasedCost(rangeWidth+1))
			require.Equal(t, pp.maxCost, pp.GetBiasedCost(math.MaxUint32))
		})
	}
}

func TestBiasedCostStaysWithinPrecedenceBand(t *testing.T) {
	// A biased cost for one precedence must never fall within another precedence's range.
	// This was possible before the overflow fix: e.g. Failed.GetBiasedCost(MaxUint32)
	// would wrap around into the Required range.
	precedences := []struct {
		name string
		p    Precedence
	}{
		{"required", Precedences.Required},
		{"default", Precedences.Default},
		{"failed", Precedences.Failed},
		{"unknown", Precedences.unknown},
	}

	for _, tc := range precedences {
		pp := tc.p.(*precedence)
		t.Run(tc.name, func(t *testing.T) {
			for _, cost := range []uint32{0, 1, 1000, math.MaxUint16, math.MaxUint32 / 2, math.MaxUint32} {
				biased := pp.GetBiasedCost(cost)
				require.GreaterOrEqual(t, biased, pp.minCost,
					"cost %d produced biased value %d below minCost %d", cost, biased, pp.minCost)
				require.LessOrEqual(t, biased, pp.maxCost,
					"cost %d produced biased value %d above maxCost %d", cost, biased, pp.maxCost)
			}
		})
	}
}
