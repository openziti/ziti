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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/storage/objectz"
	"github.com/openziti/ziti/common/datastructures"
	"github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/orcaman/concurrent-map/v2"
	"math"
	"sync"
	"time"
)

type LinkManager struct {
	linkTable      *linkTable
	idGenerator    idgen.Generator
	lock           sync.Mutex
	initialLatency time.Duration
	store          *objectz.ObjectStore[*Link]
}

func NewLinkManager(env Env) *LinkManager {
	initialLatency := config.DefaultOptionsInitialLinkLatency
	if env != nil {
		initialLatency = env.GetConfig().Network.InitialLinkLatency
	}

	result := &LinkManager{
		linkTable:      newLinkTable(),
		idGenerator:    idgen.NewGenerator(),
		initialLatency: initialLatency,
	}

	result.store = objectz.NewObjectStore[*Link](func() objectz.ObjectIterator[*Link] {
		return datastructures.IterateCMap[*Link](result.linkTable.links)
	})

	result.store.AddStringSymbol("id", func(entity *Link) *string {
		return &entity.Id
	})
	result.store.AddStringSymbol("protocol", func(entity *Link) *string {
		return &entity.Protocol
	})
	result.store.AddStringSymbol("dialAddress", func(entity *Link) *string {
		return &entity.DialAddress
	})
	result.store.AddStringSymbol("sourceRouter", func(entity *Link) *string {
		return &entity.Src.Id
	})
	result.store.AddStringSymbol("destRouter", func(entity *Link) *string {
		return &entity.DstId
	})
	result.store.AddInt64Symbol("cost", func(entity *Link) *int64 {
		val := entity.GetCost()
		return &val
	})
	result.store.AddInt64Symbol("staticCost", func(entity *Link) *int64 {
		val := int64(entity.GetStaticCost())
		return &val
	})
	result.store.AddInt64Symbol("destLatency", func(entity *Link) *int64 {
		val := entity.GetDstLatency()
		return &val
	})
	result.store.AddInt64Symbol("sourceLatency", func(entity *Link) *int64 {
		val := entity.GetSrcLatency()
		return &val
	})
	result.store.AddStringSymbol("state", func(entity *Link) *string {
		val := entity.CurrentState().Mode.String()
		return &val
	})
	result.store.AddInt64Symbol("iteration", func(entity *Link) *int64 {
		val := int64(entity.Iteration)
		return &val
	})

	return result
}

func (self *LinkManager) GetStore() *objectz.ObjectStore[*Link] {
	return self.store
}

func (self *LinkManager) BuildRouterLinks(router *Router) {
	self.linkTable.links.IterCb(func(_ string, link *Link) {
		if link.DstId == router.Id {
			router.routerLinks.Add(link, link.Src.Id)
			link.Dst.Store(router)
		}
	})
}

func (self *LinkManager) Add(link *Link) {
	self.linkTable.add(link)
	link.Src.routerLinks.Add(link, link.DstId)
	if dest := link.GetDest(); dest != nil {
		dest.routerLinks.Add(link, link.Src.Id)
	}
}

func (self *LinkManager) has(link *Link) bool {
	return self.linkTable.has(link)
}

func (self *LinkManager) ScanForDeadLinks() {
	var toRemove []*Link
	self.linkTable.links.IterCb(func(_ string, link *Link) {
		if !link.Src.Connected.Load() {
			toRemove = append(toRemove, link)
		}
	})

	for _, link := range toRemove {
		self.Remove(link)
	}
}

func (self *LinkManager) RouterReportedLink(linkId string, iteration uint32, linkProtocol, dialAddress string, src, dst *Router, dstId string) (*Link, bool) {
	self.lock.Lock()
	defer self.lock.Unlock()

	link, _ := self.Get(linkId)
	if link != nil && link.Iteration >= iteration {
		return link, false
	}

	// remove the older link before adding the new one
	if link != nil {
		log := pfxlog.Logger().
			WithField("routerId", src.Id).
			WithField("linkId", linkId).
			WithField("destRouterId", dstId).
			WithField("iteration", iteration)

		self.Remove(link)
		log.Infof("replaced link with newer iteration %v => %v", link.Iteration, iteration)
	}

	link = newLink(linkId, linkProtocol, dialAddress, self.initialLatency)
	link.Iteration = iteration
	link.Src = src
	link.Dst.Store(dst)
	link.DstId = dstId
	link.SetState(Connected)
	self.Add(link)
	return link, true
}

func (self *LinkManager) Get(linkId string) (*Link, bool) {
	return self.linkTable.get(linkId)
}

func (self *LinkManager) All() []*Link {
	return self.linkTable.all()
}

func (self *LinkManager) GetLinkMap() map[string]*Link {
	linkMap := make(map[string]*Link)
	self.linkTable.links.IterCb(func(key string, link *Link) {
		linkMap[key] = link
	})
	return linkMap
}

func (self *LinkManager) Remove(link *Link) {
	if self.linkTable.remove(link) {
		link.Src.routerLinks.Remove(link, link.DstId)
		if dest := link.GetDest(); dest != nil {
			dest.routerLinks.Remove(link, link.Src.Id)
		}
	}
}

func (self *LinkManager) ConnectedNeighborsOfRouter(router *Router) []*Router {
	neighborMap := make(map[string]*Router)

	links := router.routerLinks.GetLinks()
	for _, link := range links {
		dstRouter := link.GetDest()
		if dstRouter != nil && dstRouter.Connected.Load() && link.IsUsable() {
			if link.Src.Id != router.Id {
				neighborMap[link.Src.Id] = link.Src
			}
			if link.DstId != router.Id {
				neighborMap[link.DstId] = dstRouter
			}
		}
	}

	neighbors := make([]*Router, 0)
	for _, r := range neighborMap {
		neighbors = append(neighbors, r)
	}
	return neighbors
}

func (self *LinkManager) LeastExpensiveLink(a, b *Router) (*Link, bool) {
	var selected *Link
	var cost int64 = math.MaxInt64

	linksByRouter := a.routerLinks.GetLinksByRouter()
	links := linksByRouter[b.Id]
	for _, link := range links {
		if link.IsUsable() {
			linkCost := link.GetCost()
			if link.DstId == b.Id {
				if linkCost < cost {
					selected = link
					cost = linkCost
				}
			} else if link.Src.Id == b.Id {
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

func (self *LinkManager) MissingLinks(routers []*Router, pendingTimeout time.Duration) ([]*Link, error) {
	// When there's a flood of router connects at startup we can see the same link
	// as missing multiple times as the new link will be marked as PENDING until it's
	// connected. Give ourselves a little window to make the connection before we
	// send another dial
	pendingLimit := info.NowInMilliseconds() - pendingTimeout.Milliseconds()

	missingLinks := make([]*Link, 0)
	for _, srcR := range routers {
		if srcR.SupportsRouterLinkMgmt() {
			continue
		}

		for _, dstR := range routers {
			if srcR != dstR && len(dstR.Listeners) > 0 {
				for _, listener := range dstR.Listeners {
					if !self.hasLink(srcR, dstR, listener.GetProtocol(), pendingLimit) {
						id := idgen.NewUUIDString()
						link := newLink(id, listener.GetProtocol(), listener.GetAddress(), self.initialLatency)
						link.Src = srcR
						link.Dst.Store(dstR)
						link.DstId = dstR.Id
						missingLinks = append(missingLinks, link)
					}
				}
			}
		}
	}

	return missingLinks, nil
}

func (self *LinkManager) ClearExpiredPending(pendingTimeout time.Duration) {
	pendingLimit := info.NowInMilliseconds() - pendingTimeout.Milliseconds()

	toRemove := self.linkTable.matching(func(link *Link) bool {
		state := link.CurrentState()
		return state.Mode == Pending && state.Timestamp < pendingLimit
	})

	for _, link := range toRemove {
		self.Remove(link)
	}
}

func (self *LinkManager) hasLink(a, b *Router, linkProtocol string, pendingLimit int64) bool {
	return self.hasDirectedLink(a, b, linkProtocol, pendingLimit) || self.hasDirectedLink(b, a, linkProtocol, pendingLimit)
}

func (self *LinkManager) hasDirectedLink(a, b *Router, linkProtocol string, pendingLimit int64) bool {
	links := a.routerLinks.GetLinks()
	for _, link := range links {
		state := link.CurrentState()
		if link.Src == a && link.DstId == b.Id && link.Protocol == linkProtocol {
			if state.Mode == Connected || (state.Mode == Pending && state.Timestamp > pendingLimit) {
				return true
			}
		}
	}
	return false
}

func (self *LinkManager) LinksInMode(mode LinkMode) []*Link {
	return self.linkTable.allInMode(mode)
}

/*
 * linkTable
 */

type linkTable struct {
	links cmap.ConcurrentMap[string, *Link]
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
	lt.links.IterCb(func(_ string, link *Link) {
		links = append(links, link)
	})
	return links
}

func (lt *linkTable) allInMode(mode LinkMode) []*Link {
	links := make([]*Link, 0)
	lt.links.IterCb(func(_ string, link *Link) {
		if link.CurrentState().Mode == mode {
			links = append(links, link)
		}
	})
	return links
}

func (lt *linkTable) matching(f func(*Link) bool) []*Link {
	var links []*Link
	lt.links.IterCb(func(key string, link *Link) {
		if f(link) {
			links = append(links, link)
		}
	})
	return links
}

func (lt *linkTable) remove(link *Link) bool {
	return lt.links.RemoveCb(link.Id, func(key string, v *Link, exists bool) bool {
		return v != nil && v.Iteration == link.Iteration
	})
}
