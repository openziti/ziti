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
	"sync"
	"sync/atomic"
	"testing"

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
