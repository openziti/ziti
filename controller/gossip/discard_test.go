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

package gossip

import (
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/stretchr/testify/require"
)

// The registered test type requires link gossip, so these two masks are "usable" and "not usable" for it.
func linkGossipCaps() *capabilities.Mask[capabilities.ControllerCapability] {
	return capabilities.NewMask(capabilities.ControllerLinkGossip)
}

func noCaps() *capabilities.Mask[capabilities.ControllerCapability] {
	return capabilities.NewMask[capabilities.ControllerCapability]()
}

// encoded builds an entry value through the state type's codec, as a real sender does. A raw byte slice
// would be refused by the decode rather than by anything under test.
func encoded(t *testing.T, value string) []byte {
	t.Helper()
	b, err := encodeString(value)
	require.NoError(t, err)
	return b
}

// TestDiscard_TellsNobody is the whole point of Discard existing rather than reusing DropOwner. What the
// entries describe is still live and is about to be reported by other means, so the listener must not hear
// about it: for link state the listener removes the link from the link manager and forwards a fault to the
// routers, which would tear down every link in the fleet. Peers must not hear about it either, since a
// tombstone removes the state everywhere.
func TestDiscard_TellsNobody(t *testing.T) {
	req := require.New(t)

	store, mesh := newTestStore("peer1")
	defer store.Stop()

	listener := &mockListener{}
	st := newTestStateType(store, listener)

	req.NoError(st.Set("a", "owner1", "v1"))
	req.NoError(st.Set("b", "owner1", "v2"))
	req.NoError(st.Set("c", "owner2", "v3"))

	// Settle the writes first, so what is asserted below is the discard's doing and not a write still in
	// flight.
	req.Eventually(func() bool { return len(listener.getChanges()) == 3 }, time.Second, 5*time.Millisecond)
	changesBefore := len(listener.getChanges())
	mesh.clearSent()

	req.Equal(3, st.Discard())

	// Never, not Zero: notification is asynchronous, so a discard that did notify would need time to show
	// up, and asserting immediately would pass either way.
	req.Never(func() bool { return len(listener.getRemoves()) > 0 }, 250*time.Millisecond, 10*time.Millisecond,
		"a discard must not tell the listener the state is gone")
	req.Equal(changesBefore, len(listener.getChanges()), "a discard must not tell the listener anything")
	req.Never(func() bool { return len(mesh.getSent()) > 0 }, 100*time.Millisecond, 10*time.Millisecond,
		"a discard must not propagate, or the state would be removed cluster-wide")
}

// TestDiscard_LeavesNothingBehind: an emptied owner has to be compacted out, or it keeps being counted and
// keeps appearing in digests. The owner count is the number that reveals per-owner state accumulating.
func TestDiscard_LeavesNothingBehind(t *testing.T) {
	req := require.New(t)

	store, _ := newTestStore()
	defer store.Stop()

	st := newTestStateType(store, nil)

	req.NoError(st.Set("a", "owner1", "v1"))
	req.NoError(st.Set("b", "owner2", "v2"))

	st.Discard()

	stats := store.GetStats()
	req.Len(stats, 1)
	req.Zero(stats[0].Entries, "no entries may remain")
	req.Zero(stats[0].Tombstones, "a discard must not leave tombstones")
	req.Zero(stats[0].Owners, "an emptied owner must be compacted out")

	_, _, found := st.GetForOwner("owner1", "a")
	req.False(found)
}

// TestDiscard_AcceptsWritesAgain: unlike DropOwner, which drains an owner for good because the router is
// gone, a discard abandons state that may become usable again. The store has to refill.
func TestDiscard_AcceptsWritesAgain(t *testing.T) {
	req := require.New(t)

	store, _ := newTestStore()
	defer store.Stop()

	listener := &mockListener{}
	st := newTestStateType(store, listener)

	req.NoError(st.Set("a", "owner1", "v1"))
	st.Discard()

	req.NoError(st.Set("a", "owner1", "v2"))

	value, _, found := st.GetForOwner("owner1", "a")
	req.True(found, "an owner must not be drained by a discard")
	req.Equal("v2", value)

	req.Eventually(func() bool {
		present, known := listener.finalState("a")
		return known && present
	}, time.Second, 5*time.Millisecond, "the refilled entry must reach the listener")
}

// TestDiscardAll_CoversEveryType: gossip becoming unusable is cluster-wide, not per type.
func TestDiscardAll_CoversEveryType(t *testing.T) {
	req := require.New(t)

	store, _ := newTestStore()
	defer store.Stop()

	links := Register[string](store, StateTypeConfig[string]{
		Name: "links", RequiredCapability: capabilities.ControllerLinkGossip,
		Encode: encodeString, Decode: decodeString,
		Tombstones: true, TombstoneTTL: time.Minute,
	})
	canaries := Register[string](store, StateTypeConfig[string]{
		Name: "canaries", RequiredCapability: capabilities.ControllerLinkGossip,
		Encode: encodeString, Decode: decodeString,
	})

	req.NoError(links.Set("a", "owner1", "v1"))
	req.NoError(links.Set("b", "owner1", "v2"))
	req.NoError(canaries.Set("c", "owner1", "v3"))

	discarded := store.DiscardAll()
	req.Equal(map[string]int{"links": 2, "canaries": 1}, discarded)

	for _, stats := range store.GetStats() {
		req.Zero(stats.Entries, "type %v must be empty", stats.TypeName)
		req.Zero(stats.Owners, "type %v must have no owners left", stats.TypeName)
	}
}

// TestDiscardAll_ReportsNothingWhenEmpty: called on every transition, so it has to be cheap and quiet when
// there is nothing to do.
func TestDiscardAll_ReportsNothingWhenEmpty(t *testing.T) {
	req := require.New(t)

	store, _ := newTestStore()
	defer store.Stop()

	newTestStateType(store, nil)

	req.Empty(store.DiscardAll())
}

// TestClusterCapabilities_DiscardsWhenGossipStopsBeingUsable is the transition this exists for. When a cluster can
// no longer gossip, the routers report their links the pre-gossip way instead, and entries left standing
// would re-create links that path has since removed. The store is given up rather than deleted from, so the
// links themselves survive.
func TestClusterCapabilities_DiscardsWhenGossipStopsBeingUsable(t *testing.T) {
	req := require.New(t)

	store, _ := newTestStore()
	defer store.Stop()
	req.True(store.IsAvailable("test"), "a type starts usable until told otherwise")

	listener := &mockListener{}
	st := newTestStateType(store, listener)
	req.NoError(st.Set("a", "owner1", "v1"))
	req.NoError(st.Set("b", "owner1", "v2"))
	req.Eventually(func() bool { return len(listener.getChanges()) == 2 }, time.Second, 5*time.Millisecond)

	store.SetClusterCapabilities(noCaps())

	req.False(store.IsAvailable("test"))
	req.Zero(store.GetStats()[0].Entries, "the store must be given up")
	req.Never(func() bool { return len(listener.getRemoves()) > 0 }, 250*time.Millisecond, 10*time.Millisecond,
		"giving up the store must not tell the listener, or the links would be torn down")
}

// TestClusterCapabilities_RefillsWhenUsableAgain: an upgrade that completes, or a rollback that is undone, has to
// leave gossip working without a restart.
func TestClusterCapabilities_RefillsWhenUsableAgain(t *testing.T) {
	req := require.New(t)

	store, _ := newTestStore()
	defer store.Stop()

	st := newTestStateType(store, nil)
	store.SetClusterCapabilities(noCaps())
	store.SetClusterCapabilities(linkGossipCaps())
	req.True(store.IsAvailable("test"))

	req.NoError(st.Set("a", "owner1", "v1"))
	value, _, found := st.GetForOwner("owner1", "a")
	req.True(found, "the store must accept writes once gossip is usable again")
	req.Equal("v1", value)
}

// TestClusterCapabilities_StopsAntiEntropy: repairing towards a peer over a store that has been given up would
// pull entries back from whichever peer has not noticed yet, which is the repair mechanism perpetuating
// what it exists to fix.
func TestClusterCapabilities_StopsAntiEntropy(t *testing.T) {
	req := require.New(t)

	store, mesh := newTestStore("peer1")
	defer store.Stop()

	st := Register[string](store, StateTypeConfig[string]{
		Name: "test", RequiredCapability: capabilities.ControllerLinkGossip,
		Encode: encodeString, Decode: decodeString,
		Tombstones: true, TombstoneTTL: time.Minute,
		AntiEntropy: true, AntiEntropyInterval: 10 * time.Millisecond,
	})
	req.NoError(st.Set("a", "owner1", "v1"))

	// Anti-entropy runs while gossip is usable, which is what makes its absence below meaningful.
	req.Eventually(func() bool { return len(mesh.getSent()) > 0 }, time.Second, 5*time.Millisecond)

	store.SetClusterCapabilities(noCaps())
	mesh.clearSent()

	req.Never(func() bool { return len(mesh.getSent()) > 0 }, 250*time.Millisecond, 10*time.Millisecond,
		"anti-entropy must stop while gossip is unusable across the cluster")
}

// TestUnavailable_RefusesRouterEntries closes what giving up the store alone leaves open. A router that has
// not noticed the cluster can no longer gossip keeps sending, and taking those entries in would refill a
// store nobody is maintaining and re-create the links the pre-gossip path has since removed.
func TestUnavailable_RefusesRouterEntries(t *testing.T) {
	req := require.New(t)

	store, mesh := newTestStore("peer1")
	defer store.Stop()

	listener := &mockListener{}
	st := newTestStateType(store, listener)
	store.SetClusterCapabilities(noCaps())
	mesh.clearSent()

	store.ApplyAndBroadcast(&gossip_pb.GossipDelta{
		StoreType: "test",
		Entries: []*gossip_pb.GossipEntry{
			{Key: "a", Owner: "owner1", Version: 5, Value: encoded(t, "v1")},
		},
	})

	_, _, found := st.GetForOwner("owner1", "a")
	req.False(found, "an entry must not be taken into a store that has been given up")
	req.Zero(store.GetStats()[0].Entries)
	req.Never(func() bool { return len(mesh.getSent()) > 0 }, 250*time.Millisecond, 10*time.Millisecond,
		"a refused entry must not be passed on to peers")
	req.Never(func() bool { return len(listener.getChanges()) > 0 }, 100*time.Millisecond, 10*time.Millisecond,
		"a refused entry must not reach the listener")
}

// TestUnavailable_RefusesPeerEntries: a peer that has not noticed yet would otherwise refill this store
// through its broadcast or its answer to a digest, which is the same hole from the other side.
func TestUnavailable_RefusesPeerEntries(t *testing.T) {
	req := require.New(t)

	store, _ := newTestStore("peer1")
	defer store.Stop()

	st := newTestStateType(store, nil)
	store.SetClusterCapabilities(noCaps())

	delta := func(key string) *gossip_pb.GossipDelta {
		return &gossip_pb.GossipDelta{
			StoreType: "test",
			Entries: []*gossip_pb.GossipEntry{
				{Key: key, Owner: "owner1", Version: 5, Value: encoded(t, "v1")},
			},
		}
	}

	store.ApplyPeerDelta(delta("a"))
	store.ApplyAntiEntropyResponse(delta("b"))

	_, _, foundA := st.GetForOwner("owner1", "a")
	_, _, foundB := st.GetForOwner("owner1", "b")
	req.False(foundA, "a peer broadcast must not refill a store that has been given up")
	req.False(foundB, "a peer's digest answer must not refill it either")
	req.Zero(store.GetStats()[0].Entries)
}

// TestUnavailable_TakesEntriesAgainOnceUsable: refusal has to be a state, not a latch, or gossip never
// recovers from an upgrade without a controller restart.
func TestUnavailable_TakesEntriesAgainOnceUsable(t *testing.T) {
	req := require.New(t)

	store, _ := newTestStore("peer1")
	defer store.Stop()

	st := newTestStateType(store, nil)
	store.SetClusterCapabilities(noCaps())
	store.SetClusterCapabilities(linkGossipCaps())

	store.ApplyAndBroadcast(&gossip_pb.GossipDelta{
		StoreType: "test",
		Entries: []*gossip_pb.GossipEntry{
			{Key: "a", Owner: "owner1", Version: 5, Value: encoded(t, "v1")},
		},
	})

	value, _, found := st.GetForOwner("owner1", "a")
	req.True(found, "entries must be taken again once gossip is usable")
	req.Equal("v1", value)
}

// TestClusterCapabilities_GiveUpOnlyTheTypeThatLostItsCapability is what this rework exists for. Gossip
// features arrive in different releases, so a cluster part-way through an upgrade supports what the older
// release had while not yet supporting what the newer one adds. A store-wide answer would have to pick one,
// and giving up both would switch a shipped feature off during the upgrade after the one that shipped it.
func TestClusterCapabilities_GiveUpOnlyTheTypeThatLostItsCapability(t *testing.T) {
	req := require.New(t)

	store, _ := newTestStore()
	defer store.Stop()

	// ControllerGroupedCtrlChan stands in for a capability a later release adds.
	shipped := Register[string](store, StateTypeConfig[string]{
		Name: "shipped", RequiredCapability: capabilities.ControllerLinkGossip,
		Encode: encodeString, Decode: decodeString,
		Tombstones: true, TombstoneTTL: time.Minute,
	})
	next := Register[string](store, StateTypeConfig[string]{
		Name: "next", RequiredCapability: capabilities.ControllerGroupedCtrlChan,
		Encode: encodeString, Decode: decodeString,
		Tombstones: true, TombstoneTTL: time.Minute,
	})

	req.NoError(shipped.Set("a", "owner1", "v1"))
	req.NoError(next.Set("b", "owner1", "v2"))

	// The cluster supports the shipped feature but not the newer one.
	store.SetClusterCapabilities(capabilities.NewMask(capabilities.ControllerLinkGossip))

	req.True(store.IsAvailable("shipped"), "a capability every member has must stay usable")
	req.False(store.IsAvailable("next"), "a capability only some members have must not be usable")

	_, _, found := shipped.GetForOwner("owner1", "a")
	req.True(found, "the shipped feature's entries must survive the upgrade window")
	_, _, found = next.GetForOwner("owner1", "b")
	req.False(found, "the unsupported type must have given up its entries")
}

// TestRegister_RefusesATypeWithNoRequiredCapability: every gossip type needs every controller to
// understand it, so there is no ungated one. Leaving the field unset would otherwise make a type silently
// ignore the cluster's capabilities, which is the opposite of what the gating is for.
func TestRegister_RefusesATypeWithNoRequiredCapability(t *testing.T) {
	store, _ := newTestStore()
	defer store.Stop()

	require.Panics(t, func() {
		Register[string](store, StateTypeConfig[string]{
			Name: "ungated", Encode: encodeString, Decode: decodeString,
		})
	}, "a type registered with no required capability must fail where the mistake is made")
}
