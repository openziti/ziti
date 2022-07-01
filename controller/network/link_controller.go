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
	"github.com/openziti/fabric/controller/idgen"
	"github.com/openziti/foundation/v2/info"
	"github.com/orcaman/concurrent-map/v2"
	"math"
	"sync"
	"time"
)

type linkController struct {
	linkTable      *linkTable
	idGenerator    idgen.Generator
	lock           sync.Mutex
	initialLatency time.Duration
}

func newLinkController(options *Options) *linkController {
	initialLatency := DefaultNetworkOptionsInitialLinkLatency
	if options != nil {
		initialLatency = options.InitialLinkLatency
	}
	return &linkController{
		linkTable:      newLinkTable(),
		idGenerator:    idgen.NewGenerator(),
		initialLatency: initialLatency,
	}
}

func (linkController *linkController) add(link *Link) {
	linkController.linkTable.add(link)
	link.Src.routerLinks.Add(link, link.Dst)
	link.Dst.routerLinks.Add(link, link.Src)
}

func (linkController *linkController) has(link *Link) bool {
	return linkController.linkTable.has(link)
}

func (linkController *linkController) routerReportedLink(linkId, linkProtocol string, src, dst *Router) (*Link, bool) {
	linkController.lock.Lock()
	defer linkController.lock.Unlock()

	if link, found := linkController.get(linkId); found {
		return link, false
	}

	link := newLink(linkId, linkProtocol, linkController.initialLatency)
	link.Src = src
	link.Dst = dst
	link.addState(newLinkState(Connected))
	linkController.add(link)
	return link, true
}

func (linkController *linkController) get(linkId string) (*Link, bool) {
	return linkController.linkTable.get(linkId)
}

func (linkController *linkController) all() []*Link {
	return linkController.linkTable.all()
}

func (linkController *linkController) remove(link *Link) {
	linkController.linkTable.remove(link)
	link.Src.routerLinks.Remove(link, link.Dst)
	link.Dst.routerLinks.Remove(link, link.Src)
}

func (linkController *linkController) connectedNeighborsOfRouter(router *Router) []*Router {
	neighborMap := make(map[string]*Router)

	links := router.routerLinks.GetLinks()
	for _, link := range links {
		if link.IsUsable() {
			if link.Src != router {
				neighborMap[link.Src.Id] = link.Src
			}
			if link.Dst != router {
				neighborMap[link.Dst.Id] = link.Dst
			}
		}
	}

	neighbors := make([]*Router, 0)
	for _, r := range neighborMap {
		neighbors = append(neighbors, r)
	}
	return neighbors
}

func (linkController *linkController) leastExpensiveLink(a, b *Router) (*Link, bool) {
	var selected *Link
	var cost int64 = math.MaxInt64

	linksByRouter := a.routerLinks.GetLinksByRouter()
	links := linksByRouter[b.Id]
	for _, link := range links {
		if link.IsUsable() {
			linkCost := link.GetCost()
			if link.Dst == b {
				if linkCost < cost {
					selected = link
					cost = linkCost
				}
			} else if link.Src == b {
				if linkCost < cost {
					selected = link
					cost = linkCost
				}
			}
		}
	}

	if selected != nil {
		return selected, true
	}

	return nil, false
}

func (linkController *linkController) missingLinks(routers []*Router, pendingTimeout time.Duration) ([]*Link, error) {
	// When there's a flood of router connects at startup we can see the same link
	// as missing multiple times as the new link will be marked as PENDING until it's
	// connected. Give ourselves a little window to make the connection before we
	// send another dial
	pendingLimit := info.NowInMilliseconds() - pendingTimeout.Milliseconds()

	missingLinks := make([]*Link, 0)
	for _, srcR := range routers {
		for _, dstR := range routers {
			if srcR != dstR && len(dstR.Listeners) > 0 {
				for _, listener := range dstR.Listeners {
					if !linkController.hasLink(srcR, dstR, listener.Protocol(), pendingLimit) {
						id, err := idgen.NewUUIDString()
						if err != nil {
							return nil, err
						}
						link := newLink(id, listener.Protocol(), linkController.initialLatency)
						link.Src = srcR
						link.Dst = dstR
						missingLinks = append(missingLinks, link)
					}
				}
			}
		}
	}

	return missingLinks, nil
}

func (linkController *linkController) clearExpiredPending(pendingTimeout time.Duration) {
	pendingLimit := info.NowInMilliseconds() - pendingTimeout.Milliseconds()

	toRemove := linkController.linkTable.matching(func(link *Link) bool {
		state := link.CurrentState()
		return state != nil && state.Mode == Pending && state.Timestamp < pendingLimit
	})

	for _, link := range toRemove {
		linkController.remove(link)
	}
}

func (linkController *linkController) hasLink(a, b *Router, linkProtocol string, pendingLimit int64) bool {
	return linkController.hasDirectedLink(a, b, linkProtocol, pendingLimit) || linkController.hasDirectedLink(b, a, linkProtocol, pendingLimit)
}

func (linkController *linkController) hasDirectedLink(a, b *Router, linkProtocol string, pendingLimit int64) bool {
	links := a.routerLinks.GetLinks()
	for _, link := range links {
		state := link.CurrentState()
		if link.Src == a && link.Dst == b && state != nil && link.Protocol == linkProtocol {
			if state.Mode == Connected || (state.Mode == Pending && state.Timestamp > pendingLimit) {
				return true
			}
		}
	}
	return false
}

func (linkController *linkController) linksInMode(mode LinkMode) []*Link {
	return linkController.linkTable.allInMode(mode)
}

/*
 * linkTable
 */

type linkTable struct {
	links cmap.ConcurrentMap[*Link]
}

func newLinkTable() *linkTable {
	return &linkTable{links: cmap.New[*Link]()}
}

func (lt *linkTable) add(link *Link) {
	lt.links.Set(link.Id, link)
}

func (lt *linkTable) get(linkId string) (*Link, bool) {
	return lt.links.Get(linkId)
}

func (lt *linkTable) has(link *Link) bool {
	if _, found := lt.links.Get(link.Id); found {
		return true
	}
	return false
}

func (lt *linkTable) all() []*Link {
	links := make([]*Link, 0, lt.links.Count())
	for tuple := range lt.links.IterBuffered() {
		links = append(links, tuple.Val)
	}
	return links
}

func (lt *linkTable) allInMode(mode LinkMode) []*Link {
	links := make([]*Link, 0)
	for tuple := range lt.links.IterBuffered() {
		link := tuple.Val
		if link.CurrentState().Mode == mode {
			links = append(links, link)
		}
	}
	return links
}

func (lt *linkTable) matching(f func(*Link) bool) []*Link {
	var links []*Link
	for tuple := range lt.links.IterBuffered() {
		if f(tuple.Val) {
			links = append(links, tuple.Val)
		}
	}
	return links
}

func (lt *linkTable) remove(link *Link) {
	lt.links.Remove(link.Id)
}
