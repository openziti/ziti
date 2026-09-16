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

package network

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// testEpoch builds an epoch whose ordering against another is decided by the caller, rather than by when the
// test happened to run. Epochs are compared as raw bytes, so this is the same comparison production makes.
func testEpoch(ordinal byte) []byte {
	epoch := make([]byte, 16)
	epoch[0] = ordinal
	return epoch
}

// TestCanary_RestartedSequenceWins covers what a router restart depends on. A canary entry is versioned by the
// router's canary sequence, and the store refuses anything not newer, so the restarted router's canaries have to
// start above wherever its predecessor's finished. The router seeds the sequence from its epoch to guarantee
// that, since the epoch advances by the whole elapsed wall time while the sequence advances once per tick.
//
// Everything the staleness checks read comes from the canary, so a restarted router whose canaries lose leaves
// them comparing this controller's live state against a dead process's hashes, counts and sent versions.
func TestCanary_RestartedSequenceWins(t *testing.T) {
	const routerId = "r1"

	previousEpoch := testEpoch(1)
	currentEpoch := testEpoch(2)

	// Representative of what the seeding produces: the new lifetime's first sequence exceeds the old
	// lifetime's last, because the epoch moved on by more than the old lifetime could tick.
	const previousLast = uint64(50_000)
	const currentFirst = uint64(60_000)

	t.Run("the restarted sequence replaces the departed lifetime's canary", func(t *testing.T) {
		_, network, _ := newConnectTestNetwork(t)

		require.NoError(t, network.CanaryGossipType.SetWithVersion(routerId, routerId, previousLast,
			&CanaryValue{Seq: previousLast, Epoch: previousEpoch}))

		require.NoError(t, network.CanaryGossipType.SetWithVersion(routerId, routerId, currentFirst,
			&CanaryValue{Seq: currentFirst, Epoch: currentEpoch}))

		held, ok := network.GetCanaryValueForRouter(routerId)
		require.True(t, ok)
		require.Equal(t, currentFirst, held.Seq)
		require.Equal(t, currentEpoch, held.Epoch)
	})

	// The reason the entry is not dropped when a newer epoch is seen. An absent key is a create, which the
	// store accepts whatever its version, so a canary still in flight from the departed lifetime would be
	// taken in place of the one it had already lost to. It arrives that way readily: a peer's snapshot on
	// connect carries every store, canaries included, and peers act on a router's hello at different times.
	t.Run("a stale canary still in flight loses rather than being taken", func(t *testing.T) {
		_, network, _ := newConnectTestNetwork(t)

		require.NoError(t, network.CanaryGossipType.SetWithVersion(routerId, routerId, previousLast,
			&CanaryValue{Seq: previousLast, Epoch: previousEpoch}))
		require.NoError(t, network.CanaryGossipType.SetWithVersion(routerId, routerId, currentFirst,
			&CanaryValue{Seq: currentFirst, Epoch: currentEpoch}))

		// What the router's hello carries when it comes back. This must not clear the entry.
		network.HandleRouterEpoch(routerId, currentEpoch)

		// The departed lifetime's canary, arriving late.
		require.NoError(t, network.CanaryGossipType.SetWithVersion(routerId, routerId, previousLast,
			&CanaryValue{Seq: previousLast, Epoch: previousEpoch}))

		held, ok := network.GetCanaryValueForRouter(routerId)
		require.True(t, ok, "the current lifetime's canary must still be held")
		require.Equal(t, currentFirst, held.Seq, "a canary from a lifetime that ended must not be taken")
		require.Equal(t, currentEpoch, held.Epoch)
	})

	// A reconnect that is not a restart carries the same epoch, so there is nothing to sweep and the
	// controller keeps tracking the sequence it already has.
	t.Run("the same epoch leaves the canary alone", func(t *testing.T) {
		_, network, _ := newConnectTestNetwork(t)

		require.NoError(t, network.CanaryGossipType.SetWithVersion(routerId, routerId, currentFirst,
			&CanaryValue{Seq: currentFirst, Epoch: currentEpoch}))

		network.HandleRouterEpoch(routerId, currentEpoch)

		held, ok := network.GetCanaryValueForRouter(routerId)
		require.True(t, ok, "a reconnect on the same epoch must not discard the canary")
		require.Equal(t, currentFirst, held.Seq)
	})
}
