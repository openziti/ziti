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
		DstId: r1.Id,
	}
	l0.Src.Store(r0)
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

// Test_BuildRouterLinks_refreshesStaleSrc covers the gossip scenario: a link can
// be created referencing a database-loaded (disconnected) source router when its
// gossip entry arrives before the router connects to this controller. When the
// router then connects, BuildRouterLinks must repoint the link at the connected
// router object and index the link under it.
func Test_BuildRouterLinks_refreshesStaleSrc(t *testing.T) {
	req := require.New(t)
	lm := NewLinkManager(nil)

	srcDb := NewRouter("r0", "", "", 0, true) // database-loaded, not connected
	dst := NewRouter("r1", "", "", 0, true)
	dst.Connected.Store(true)

	link := NewTestLink("l0", srcDb, dst)
	lm.Add(link)
	req.Same(srcDb, link.GetSrc())

	// r0 connects: a fresh, connected router object with the same id.
	srcConn := NewRouter("r0", "", "", 0, true)
	srcConn.Connected.Store(true)
	lm.BuildRouterLinks(srcConn)

	req.Same(srcConn, link.GetSrc(), "link source repointed to the connected router object")

	connLinks := srcConn.routerLinks.GetLinks()
	req.Len(connLinks, 1, "link indexed under the connected source router")
	req.Same(link, connLinks[0])

	// The link still removes cleanly (index stays consistent after the refresh).
	lm.Remove(link)
	req.False(lm.has(link))
	req.Len(srcConn.routerLinks.GetLinks(), 0)
}

// Test_RouterReportedLink_srcConcurrentAccess is the regression guard for the
// data race the AtomicValue change fixes: the connect-time source repoint
// (BuildRouterLinks) Stores link.Src while routing/path code Loads it. With Src
// as a plain pointer this raced; as an AtomicValue it is safe. Run under -race.
func Test_RouterReportedLink_srcConcurrentAccess(t *testing.T) {
	lm := NewLinkManager(nil)

	dst := NewRouter("r1", "", "", 0, true)
	dst.Connected.Store(true)
	link := NewTestLink("l0", NewRouter("r0", "", "", 0, true), dst)
	lm.Add(link)

	// Two connected r0 objects so each BuildRouterLinks call repoints (src != router),
	// keeping the Store path hot against the concurrent readers.
	srcA := NewRouter("r0", "", "", 0, true)
	srcA.Connected.Store(true)
	srcB := NewRouter("r0", "", "", 0, true)
	srcB.Connected.Store(true)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if s := link.GetSrc(); s != nil {
						_ = s.Id
					}
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			if i%2 == 0 {
				lm.BuildRouterLinks(srcA)
			} else {
				lm.BuildRouterLinks(srcB)
			}
		}
		close(stop)
	}()

	wg.Wait()
}
