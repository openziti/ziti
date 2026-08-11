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

// Test_linkTable_forRouter covers the router-id index that lets per-router work avoid scanning the whole
// link table: a link is indexed under both of its endpoints, and removing it unindexes it from both.
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

// Test_linkTable_indexStaysConsistentUnderChurn is the leak guard for the router-id index: it must hold
// exactly the links the table holds, and nothing of a link that has been removed. The index keys off a
// link's endpoint ids, so any future change that altered a link's endpoint identity after it was added, or
// that bypassed the add/remove path, would silently strand entries here and keep dead links reachable.
// Rather than assert that indirectly, this compares the index against the table for every router after a
// mesh has been built, torn down in part, and had links replaced by higher iterations.
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

	// Connect-time repointing replaces the Router objects a link references, which must not disturb the
	// index, since it is keyed on ids rather than objects.
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

// Test_linkTable_removeKeepsAReplacementIndexed covers the guard on the index cleanup: a removal must never
// erase an index entry that refers to a different link than the one being removed.
//
// Add and Remove now serialize on the link's id, so a replacement cannot index itself while a removal of the
// older link is in flight, which makes this belt-and-braces rather than the only thing standing between the
// index and a lost entry. It is kept because erasing the wrong entry is invisible in the table, which still
// holds the link, and surfaces only later: the index is what BuildRouterLinks reads, so on the next reconnect
// the link is never found, its destination is never repointed, and it is never indexed on the router.
func Test_linkTable_removeKeepsAReplacementIndexed(t *testing.T) {
	req := require.New(t)

	src := NewRouter("r0", "", "", 0, true)
	dst := NewRouter("r1", "", "", 0, true)

	lt := newLinkTable()
	link := NewTestLink("l0", src, dst)
	lt.add(link)

	replacement := NewTestLink("l0", src, dst)
	replacement.Iteration = link.Iteration + 1

	// An index holding a different link than the one being removed.
	for _, routerId := range []string{src.Id, dst.Id} {
		index, found := lt.byRouter.Get(routerId)
		req.True(found)
		index.Set(replacement.Id, replacement)
	}

	req.True(lt.remove(link), "the link's own iteration is what the primary removal matches on")

	// Asserted against the index itself rather than forRouter, which reads the table as authoritative and
	// so would filter the replacement out for a reason unrelated to this guard.
	for _, routerId := range []string{src.Id, dst.Id} {
		index, found := lt.byRouter.Get(routerId)
		req.True(found)
		indexed, ok := index.Get(replacement.Id)
		req.True(ok, "the replacement's index entry must survive the older link's removal under %v", routerId)
		req.Same(replacement, indexed)
	}
}

// Test_linkTable_forRouterTreatsTheTableAsAuthoritative: the index is a cache of the link table written in a
// separate step, so a per-router query must not hand back a link the table no longer holds. Without this a
// removed link stays reachable through BuildRouterLinks and can be routed over.
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
}

// Test_LinkManager_AddAndRemoveSerializeOnLinkId asserts the mutual exclusion the index depends on.
//
// The table and its per-router index are written in sequence, so a removal that runs between an add's two
// writes removes the table entry, finds nothing yet in the index, and lets the add index a link the table no
// longer holds, which nothing then cleans up. The window is two map writes wide, so a stress test does not
// reach it reliably; what is checkable is the property that closes it, that a change to a link cannot begin
// while another change to the same link is in flight.
func Test_LinkManager_AddAndRemoveSerializeOnLinkId(t *testing.T) {
	req := require.New(t)
	lm := NewLinkManager(nil)

	src := NewRouter("r0", "", "", 0, true)
	dst := NewRouter("r1", "", "", 0, true)
	link := NewTestLink("l0", src, dst)

	// Stand in for a change to this link already in progress.
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
