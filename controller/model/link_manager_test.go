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

package model

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A simple test to check for failure of alignment on atomic operations for 64 bit variables in a struct
func Test64BitAlignment(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("One of the variables that was tested is not properly 64-bit aligned.")
		}
	}()

	link := Link{}

	atomic.LoadInt64(&link.SrcLatency)
	atomic.LoadInt64(&link.DstLatency)
	atomic.LoadInt64(&link.Cost)
}

func TestLifecycle(t *testing.T) {
	linkController := NewLinkManager(nil)

	r0 := NewRouter("r0", "", "", 0, true)
	r1 := NewRouter("r1", "", "", 0, true)
	l0 := &Link{
		Id:    "l0",
		Src:   r0,
		DstId: r1.Id,
	}
	l0.Dst.Store(r1)

	linkController.Add(l0)
	assert.True(t, linkController.has(l0))

	links := r0.routerLinks.GetLinks()
	assert.Equal(t, 1, len(links))
	assert.Equal(t, l0, links[0])

	links = r1.routerLinks.GetLinks()
	assert.Equal(t, 1, len(links))
	assert.Equal(t, l0, links[0])

	linkController.Remove(l0)
	assert.False(t, linkController.has(l0))

	links = r0.routerLinks.GetLinks()
	assert.Equal(t, 0, len(links))

	links = r1.routerLinks.GetLinks()
	assert.Equal(t, 0, len(links))
}

func TestNeighbors(t *testing.T) {
	linkController := NewLinkManager(nil)

	r0 := NewRouterForTest("r0", "", nil, nil, 0, true)
	r1 := NewRouterForTest("r1", "", nil, nil, 0, true)
	l0 := NewTestLink("l0", r0, r1)
	l0.SetState(Connected)
	linkController.Add(l0)

	neighbors := linkController.ConnectedNeighborsOfRouter(r0)
	assert.Equal(t, 1, len(neighbors))
	assert.Equal(t, r1, neighbors[0])
}

// Test_RouterReportedLink_serializesAndStableOnStaleReport covers two
// RouterReportedLink guarantees: concurrent reports for the same link serialize
// through the striped per-link lock (no corruption under -race, and the highest
// reported iteration wins regardless of arrival order), and a stale/same-iteration
// report is a no-op that returns the existing link without replacing it or its
// source/dest routers.
func Test_RouterReportedLink_serializesAndStableOnStaleReport(t *testing.T) {
	req := require.New(t)
	lm := NewLinkManager(nil)

	src := NewRouter("r0", "", "", 0, true)
	src.Connected.Store(true)
	dst := NewRouter("r1", "", "", 0, true)
	dst.Connected.Store(true)

	report := func(iteration uint32) *ctrl_pb.RouterLinks_RouterLink {
		return &ctrl_pb.RouterLinks_RouterLink{
			Id:           "l0",
			DestRouterId: dst.Id,
			LinkProtocol: "tls",
			DialAddress:  "tcp:localhost:1234",
			Iteration:    iteration,
		}
	}

	link, created := lm.RouterReportedLink(report(1), src, dst)
	req.True(created)
	req.NotNil(link)
	req.Same(src, link.Src)
	req.Same(dst, link.GetDest())

	// A stale/same-iteration report returns the existing link unchanged: created is
	// false, the same link object comes back, and the source router is not swapped
	// even though a different (also connected) router object with the same id reports.
	otherSrc := NewRouter("r0", "", "", 0, true)
	otherSrc.Connected.Store(true)
	again, created2 := lm.RouterReportedLink(report(1), otherSrc, dst)
	req.False(created2)
	req.Same(link, again, "same-iteration report returns the existing link")
	req.Same(src, again.Src, "same-iteration report must not replace the source router")

	// Concurrent reports for the same link serialize through the per-link lock. With
	// mixed iterations arriving in arbitrary order the highest iteration must win
	// (lower ones are rejected as stale once it lands), and the table holds exactly
	// one link. Run under -race to catch unsynchronized access.
	const maxIteration = 32
	var wg sync.WaitGroup
	for i := uint32(1); i <= maxIteration; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			lm.RouterReportedLink(report(i), src, dst)
		}()
	}
	wg.Wait()

	got, ok := lm.Get("l0")
	req.True(ok)
	req.Equal(uint32(maxIteration), got.Iteration, "highest reported iteration wins regardless of arrival order")
	req.Len(lm.All(), 1, "exactly one link remains in the table")
}

// Test_linkTable_forRouter asserts a link is indexed under both of its endpoints, and unindexed from both
// when removed.
func Test_linkTable_forRouter(t *testing.T) {
	req := require.New(t)
	lm := NewLinkManager(nil)

	r0 := NewRouterForTest("r0", "", nil, nil, 0, true)
	r1 := NewRouterForTest("r1", "", nil, nil, 0, true)
	r2 := NewRouterForTest("r2", "", nil, nil, 0, true)

	l01 := NewTestLink("l01", r0, r1)
	l02 := NewTestLink("l02", r0, r2)
	lm.Add(l01)
	lm.Add(l02)

	req.ElementsMatch([]*Link{l01, l02}, lm.linkTable.forRouter("r0"), "indexed under the source router")
	req.Equal([]*Link{l01}, lm.linkTable.forRouter("r1"), "indexed under the destination router")
	req.Equal([]*Link{l02}, lm.linkTable.forRouter("r2"))
	req.Empty(lm.linkTable.forRouter("no-such-router"))

	lm.Remove(l01)
	req.Equal([]*Link{l02}, lm.linkTable.forRouter("r0"), "removal unindexes from the source")
	req.Empty(lm.linkTable.forRouter("r1"), "removal unindexes from the destination")
}

// Test_linkTable_indexStaysConsistentUnderChurn is the leak guard for the router-id index: after a mesh is
// built, partly torn down, and has links replaced by higher iterations, the index must hold exactly what the
// table holds for every router. A change that altered a link's endpoint ids after it was added, or that
// bypassed the add/remove path, would strand entries here and keep dead links reachable.
func Test_linkTable_indexStaysConsistentUnderChurn(t *testing.T) {
	req := require.New(t)
	lm := NewLinkManager(nil)

	const routerCount = 12
	routers := make([]*Router, routerCount)
	for i := range routers {
		routers[i] = NewRouterForTest(fmt.Sprintf("r%02d", i), "", nil, nil, 0, true)
	}

	linkId := func(i, j int) string { return fmt.Sprintf("l-%02d-%02d", i, j) }

	// A full mesh, so every router carries links at both ends.
	for i := 0; i < routerCount; i++ {
		for j := i + 1; j < routerCount; j++ {
			lm.Add(NewTestLink(linkId(i, j), routers[i], routers[j]))
		}
	}

	// Remove a scattering of links.
	for i := 0; i < routerCount; i += 3 {
		for j := i + 1; j < routerCount; j += 2 {
			if link, found := lm.Get(linkId(i, j)); found {
				lm.Remove(link)
			}
		}
	}

	// Replace some links with a higher iteration, which removes and re-adds the same link id.
	for i := 1; i < routerCount; i += 4 {
		j := i + 1
		lm.RouterReportedLink(&ctrl_pb.RouterLinks_RouterLink{
			Id:           linkId(i, j),
			DestRouterId: routers[j].Id,
			LinkProtocol: "tls",
			DialAddress:  "tcp:localhost:1234",
			Iteration:    7,
		}, routers[i], routers[j])
	}

	// Connect-time repointing replaces the Router objects a link references. The index is keyed on ids, so
	// this must not disturb it.
	for i := range routers {
		fresh := NewRouterForTest(routers[i].Id, "", nil, nil, 0, true)
		lm.BuildRouterLinks(fresh)
	}

	// The index must agree with the table, per router, in both directions.
	for _, router := range routers {
		expected := map[string]*Link{}
		for _, link := range lm.All() {
			if link.Src.Id == router.Id || link.DstId == router.Id {
				expected[link.Id] = link
			}
		}

		indexed := map[string]*Link{}
		for _, link := range lm.linkTable.forRouter(router.Id) {
			indexed[link.Id] = link
		}

		req.Equal(expected, indexed, "index and table disagree for router %v", router.Id)
	}
}

// Test_linkTable_removeKeepsAReplacementIndexed covers the guard on index cleanup: a removal must not
// unindex an id the table holds a link under again. Add and Remove serialize on the link id, so this is
// belt-and-braces, kept because dropping the id is invisible in the table and surfaces only when a
// per-router query later fails to find the link.
func Test_linkTable_removeKeepsAReplacementIndexed(t *testing.T) {
	req := require.New(t)

	src := NewRouter("r0", "", "", 0, true)
	dst := NewRouter("r1", "", "", 0, true)

	lt := newLinkTable()
	link := NewTestLink("l0", src, dst)
	lt.add(link)

	// The interleave the guard exists for: a replacement takes the table slot and indexes itself between a
	// removal's table write and its unindex. Driven through unindex directly, since remove's iteration
	// check refuses the primary removal once the replacement is in the table.
	replacement := NewTestLink("l0", src, dst)
	replacement.Iteration = link.Iteration + 1
	lt.links.Set(replacement.Id, replacement)

	lt.unindex(link)

	for _, routerId := range []string{src.Id, dst.Id} {
		req.Equal([]*Link{replacement}, lt.forRouter(routerId),
			"the replacement must stay reachable under %v", routerId)
	}

	// With nothing live under the id, the same call must drop it rather than strand it. forRouter cannot
	// tell the two apart, so this asserts against the index.
	lt.links.Remove(replacement.Id)
	lt.unindex(link)

	for _, routerId := range []string{src.Id, dst.Id} {
		index, found := lt.byRouter.Get(routerId)
		req.True(found)
		req.False(index.Contains(link.Id), "a removed link's id must not be left indexed under %v", routerId)
	}
}

// Test_linkTable_forRouterTreatsTheTableAsAuthoritative asserts a per-router query does not hand back a link
// the table no longer holds. The index is written in a separate step, so it can briefly disagree.
func Test_linkTable_forRouterTreatsTheTableAsAuthoritative(t *testing.T) {
	req := require.New(t)

	src := NewRouter("r0", "", "", 0, true)
	dst := NewRouter("r1", "", "", 0, true)

	lt := newLinkTable()
	link := NewTestLink("l0", src, dst)
	lt.add(link)
	req.Len(lt.forRouter(src.Id), 1)

	// A stale index entry: gone from the table, still indexed.
	lt.links.Remove(link.Id)

	req.Empty(lt.forRouter(src.Id), "a link the table no longer holds must not be returned")
	req.Empty(lt.forRouter(dst.Id), "nor under its other endpoint")

	// The same id resolving to a new link is not staleness, and must be returned.
	replacement := NewTestLink("l0", src, dst)
	replacement.Iteration = link.Iteration + 1
	lt.links.Set(replacement.Id, replacement)

	req.Equal([]*Link{replacement}, lt.forRouter(src.Id), "a replaced link resolves to whatever the table holds")
	req.Equal([]*Link{replacement}, lt.forRouter(dst.Id))
}

// Test_LinkManager_BuildRouterLinksSerializesOnLinkId asserts the connect-time pairing takes the same
// stripe every other change to a link takes. Without it a removal that reads no destination can complete
// between this reading the table and adding to the router's link set, leaving the link in that set after
// the table has dropped it, where path computation still routes over it.
func Test_LinkManager_BuildRouterLinksSerializesOnLinkId(t *testing.T) {
	req := require.New(t)
	lm := NewLinkManager(nil)

	src := NewRouter("r0", "", "", 0, true)
	dst := NewRouter("r1", "", "", 0, true)
	link := NewTestLink("l0", src, dst)
	link.Dst.Store(nil)
	lm.Add(link)

	// Stands in for a removal of this link already in progress.
	unlock := lm.linkLocks.LockFor(link.Id)

	built := make(chan struct{})
	go func() {
		lm.BuildRouterLinks(dst)
		close(built)
	}()

	select {
	case <-built:
		req.Fail("the link build paired a link while another change to it was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	unlock()

	select {
	case <-built:
	case <-time.After(5 * time.Second):
		req.Fail("the link build did not proceed once the link's stripe was released")
	}
	req.Same(dst, link.GetDest(), "the link must be paired once the stripe is free")
	req.Len(dst.GetLinks(), 1)
}

// Test_LinkManager_BuildRouterLinksSkipsUnresolvedIds: the index can name an id the table no longer holds,
// which is the state a removal leaves behind when it keeps an id a replacement may have taken. The pairing
// resolves ids under the stripe and must skip the ones that resolve to nothing, rather than adding a link
// the table has dropped to the router's link set, where nothing would clean it up.
func Test_LinkManager_BuildRouterLinksSkipsUnresolvedIds(t *testing.T) {
	req := require.New(t)
	lm := NewLinkManager(nil)

	src := NewRouter("r0", "", "", 0, true)
	dst := NewRouter("r1", "", "", 0, true)
	link := NewTestLink("l0", src, dst)
	link.Dst.Store(nil)
	lm.Add(link)

	// Gone from the table, still indexed.
	lm.linkTable.links.Remove(link.Id)
	req.Equal([]string{link.Id}, lm.linkTable.linkIdsForRouter(dst.Id), "the id must still be indexed")

	lm.BuildRouterLinks(dst)

	req.Nil(link.GetDest(), "a link the table no longer holds must not be paired")
	req.Empty(dst.GetLinks(), "nor added to the router's link set")
}

// Test_LinkManager_RouterDeletedDropsTheIndex: a router id is only ever added to the index, so without a
// drop on delete every router that has ever had a link is held for the controller's lifetime. The links
// themselves are left alone, since a deleted router's peers still hold theirs.
func Test_LinkManager_RouterDeletedDropsTheIndex(t *testing.T) {
	req := require.New(t)
	lm := NewLinkManager(nil)

	gone := NewRouter("r0", "", "", 0, true)
	peer := NewRouter("r1", "", "", 0, true)
	link := NewTestLink("l0", gone, peer)
	lm.Add(link)

	req.Len(lm.LinksForRouter(gone.Id), 1)
	req.Len(lm.LinksForRouter(peer.Id), 1)

	lm.RouterDeleted(gone.Id)

	_, found := lm.linkTable.byRouter.Get(gone.Id)
	req.False(found, "the deleted router's index must be dropped, not just emptied")
	req.Empty(lm.LinksForRouter(gone.Id))

	req.Len(lm.LinksForRouter(peer.Id), 1, "the other endpoint's index must be untouched")
	req.True(lm.has(link), "dropping an index must not remove the link itself")

	// A report still in flight recreates the entry. Nothing reads it, since a deleted router cannot connect.
	lm.Add(link)
	req.Len(lm.LinksForRouter(peer.Id), 1, "the peer's index must not be double-counted by the recreate")
}

// Test_LinkManager_AddAndRemoveSerializeOnLinkId asserts a change to a link cannot begin while another
// change to the same link is in flight. That is what keeps a removal from running between an add's table and
// index writes, which would leave the link indexed but untabled. The window is two map writes wide, so the
// property is asserted directly rather than by stressing it.
func Test_LinkManager_AddAndRemoveSerializeOnLinkId(t *testing.T) {
	req := require.New(t)
	lm := NewLinkManager(nil)

	src := NewRouter("r0", "", "", 0, true)
	dst := NewRouter("r1", "", "", 0, true)
	link := NewTestLink("l0", src, dst)

	// Stands in for a change to this link already in progress.
	unlock := lm.linkLocks.LockFor(link.Id)

	added := make(chan struct{})
	go func() {
		lm.Add(link)
		close(added)
	}()

	select {
	case <-added:
		req.Fail("Add published while another change to the same link was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	unlock()

	select {
	case <-added:
	case <-time.After(5 * time.Second):
		req.Fail("Add did not proceed once the link's stripe was released")
	}
	req.True(lm.has(link))

	// Remove takes the same stripe, so it is serialized against an add for the same link either way round.
	unlock = lm.linkLocks.LockFor(link.Id)
	removed := make(chan struct{})
	go func() {
		lm.Remove(link)
		close(removed)
	}()

	select {
	case <-removed:
		req.Fail("Remove retracted while another change to the same link was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	unlock()

	select {
	case <-removed:
	case <-time.After(5 * time.Second):
		req.Fail("Remove did not proceed once the link's stripe was released")
	}
	req.False(lm.has(link))
}
