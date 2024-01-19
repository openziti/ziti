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
	"encoding/json"
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/storage/objectz"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller/idgen"
	"github.com/orcaman/concurrent-map/v2"
	"math"
	"strings"
	"sync"
	"time"
)

type linkController struct {
	linkTable      *linkTable
	idGenerator    idgen.Generator
	lock           sync.Mutex
	initialLatency time.Duration
	store          *objectz.ObjectStore[*Link]
}

func newLinkController(options *Options) *linkController {
	initialLatency := DefaultOptionsInitialLinkLatency
	if options != nil {
		initialLatency = options.InitialLinkLatency
	}

	result := &linkController{
		linkTable:      newLinkTable(),
		idGenerator:    idgen.NewGenerator(),
		initialLatency: initialLatency,
	}

	result.store = objectz.NewObjectStore[*Link](func() objectz.ObjectIterator[*Link] {
		return IterateCMap[*Link](result.linkTable.links)
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
		if state := entity.CurrentState(); state != nil {
			val := state.Mode.String()
			return &val
		}
		return nil
	})
	result.store.AddInt64Symbol("iteration", func(entity *Link) *int64 {
		val := int64(entity.Iteration)
		return &val
	})

	return result
}

func (linkController *linkController) buildRouterLinks(router *Router) {
	for entry := range linkController.linkTable.links.IterBuffered() {
		link := entry.Val
		if link.DstId == router.Id {
			router.routerLinks.Add(link, link.Src.Id)
			link.Dst.Store(router)
		}
	}
}

func (linkController *linkController) add(link *Link) {
	linkController.linkTable.add(link)
	link.Src.routerLinks.Add(link, link.DstId)
	if dest := link.GetDest(); dest != nil {
		dest.routerLinks.Add(link, link.Src.Id)
	}
}

func (linkController *linkController) has(link *Link) bool {
	return linkController.linkTable.has(link)
}

func (linkController *linkController) routerReportedLink(linkId string, iteration uint32, linkProtocol, dialAddress string, src, dst *Router, dstId string) (*Link, bool) {
	linkController.lock.Lock()
	defer linkController.lock.Unlock()

	link, _ := linkController.get(linkId)
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

		linkController.remove(link)
		log.Infof("replaced link with newer iteration %v => %v", link.Iteration, iteration)
	}

	link = newLink(linkId, linkProtocol, dialAddress, linkController.initialLatency)
	link.Iteration = iteration
	link.Src = src
	link.Dst.Store(dst)
	link.DstId = dstId
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
	if linkController.linkTable.remove(link) {
		link.Src.routerLinks.Remove(link, link.DstId)
		if dest := link.GetDest(); dest != nil {
			dest.routerLinks.Remove(link, link.Src.Id)
		}
	}
}

func (linkController *linkController) connectedNeighborsOfRouter(router *Router) []*Router {
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

func (linkController *linkController) leastExpensiveLink(a, b *Router) (*Link, bool) {
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
		if srcR.HasCapability(ctrl_pb.RouterCapability_LinkManagement) {
			continue
		}

		for _, dstR := range routers {
			if srcR != dstR && len(dstR.Listeners) > 0 {
				for _, listener := range dstR.Listeners {
					if !linkController.hasLink(srcR, dstR, listener.GetProtocol(), pendingLimit) {
						id := idgen.NewUUIDString()
						link := newLink(id, listener.GetProtocol(), listener.GetAddress(), linkController.initialLatency)
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
		if link.Src == a && link.DstId == b.Id && state != nil && link.Protocol == linkProtocol {
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

func (self *linkController) ValidateRouterLinks(n *Network, router *Router, cb LinkValidationCallback) {
	request := &ctrl_pb.InspectRequest{RequestedValues: []string{"links"}}
	resp := &ctrl_pb.InspectResponse{}
	respMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Minute).SendForReply(router.Control)
	if err = protobufs.TypedResponse(resp).Unmarshall(respMsg, err); err != nil {
		self.reportRouterLinksError(router, err, cb)
		return
	}

	var linkDetails *inspect.LinksInspectResult
	for _, val := range resp.Values {
		if val.Name == "links" {
			if err = json.Unmarshal([]byte(val.Value), &linkDetails); err != nil {
				self.reportRouterLinksError(router, err, cb)
				return
			}
		}
	}

	if linkDetails == nil {
		if len(resp.Errors) > 0 {
			err = errors.New(strings.Join(resp.Errors, ","))
			self.reportRouterLinksError(router, err, cb)
			return
		}
		self.reportRouterLinksError(router, errors.New("no link details returned from router"), cb)
		return
	}

	linkMap := map[string]*Link{}

	for entry := range self.linkTable.links.IterBuffered() {
		linkMap[entry.Key] = entry.Val
	}

	result := &mgmt_pb.RouterLinkDetails{
		RouterId:        router.Id,
		RouterName:      router.Name,
		ValidateSuccess: true,
	}

	for _, link := range linkDetails.Links {
		detail := &mgmt_pb.RouterLinkDetail{
			LinkId:       link.Id,
			RouterState:  mgmt_pb.LinkState_LinkEstablished,
			DestRouterId: link.Dest,
			Dialed:       link.Dialed,
		}
		detail.DestConnected = n.ConnectedRouter(link.Dest)
		if _, found := linkMap[link.Id]; found {
			detail.CtrlState = mgmt_pb.LinkState_LinkEstablished
			detail.IsValid = detail.DestConnected
		} else {
			detail.CtrlState = mgmt_pb.LinkState_LinkUnknown
			detail.IsValid = !detail.DestConnected
		}
		delete(linkMap, link.Id)
		result.LinkDetails = append(result.LinkDetails, detail)
	}

	for _, link := range linkMap {
		related := false
		dest := ""
		if link.Src.Id == router.Id {
			related = true
			dest = link.DstId
		} else if link.DstId == router.Id {
			related = true
			dest = link.Src.Id
		}

		if related {
			detail := &mgmt_pb.RouterLinkDetail{
				LinkId:        link.Id,
				CtrlState:     mgmt_pb.LinkState_LinkEstablished,
				DestConnected: n.ConnectedRouter(dest),
				RouterState:   mgmt_pb.LinkState_LinkUnknown,
				IsValid:       false,
				DestRouterId:  dest,
				Dialed:        link.Src.Id == router.Id,
			}
			result.LinkDetails = append(result.LinkDetails, detail)
		}
	}

	cb(result)
}

func (self *linkController) reportRouterLinksError(router *Router, err error, cb LinkValidationCallback) {
	result := &mgmt_pb.RouterLinkDetails{
		RouterId:        router.Id,
		RouterName:      router.Name,
		ValidateSuccess: false,
		Message:         err.Error(),
	}
	cb(result)
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

func (lt *linkTable) remove(link *Link) bool {
	return lt.links.RemoveCb(link.Id, func(key string, v *Link, exists bool) bool {
		return v != nil && v.Iteration == link.Iteration
	})
}
