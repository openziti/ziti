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
	"strings"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/model"
)

// GossipValidationCallback receives one result per validated component (router or controller).
type GossipValidationCallback func(detail *mgmt_pb.GossipValidationDetails)

// ValidateGossip validates link-gossip consistency. For each matched router it
// diffs the router's link registry against the router's own gossip store and
// against this controller's gossip store, surfacing registry<->gossip desync and
// router->controller propagation gaps. When validateCtrl is set, it also diffs
// this controller's link manager against its gossip store (orphans / phantoms).
func (network *Network) ValidateGossip(filter string, validateCtrl bool, cb GossipValidationCallback) (int64, func(), error) {
	result, err := network.Router.BaseList(filter)
	if err != nil {
		return 0, nil, err
	}

	count := int64(len(result.Entities))
	if validateCtrl {
		count++
	}

	sem := concurrenz.NewSemaphore(10)

	// Nothing publishes link gossip while the cluster cannot carry it: the routers report links the
	// pre-gossip way and every gossip store has been given up. There is no divergence to find, and checking
	// anyway would report every link as missing from gossip and every router as holding nothing. What the
	// links are doing in that state is validateRouterLinks' subject, not this one's.
	clusterGossips := network.ClusterSupports(capabilities.ControllerLinkGossip)

	evalF := func() {
		// Routers are checked first, and the controller last, because the controller check needs to know
		// which routers publish gossip and only the routers can say.
		var wg sync.WaitGroup
		var lock sync.Mutex
		gossipingRouters := map[string]struct{}{}

		for _, router := range result.Entities {
			connectedRouter := network.GetConnectedRouter(router.Id)
			if connectedRouter == nil {
				network.reportRouterGossipError(router, errors.New("router not connected"), cb)
				continue
			}

			if !clusterGossips {
				network.reportRouterGossipSkipped(connectedRouter, "cluster cannot carry link gossip", cb)
				continue
			}

			sem.Acquire()
			wg.Add(1)
			go func() {
				defer sem.Release()
				defer wg.Done()
				if network.ValidateRouterGossip(connectedRouter, cb) {
					lock.Lock()
					gossipingRouters[connectedRouter.Id] = struct{}{}
					lock.Unlock()
				}
			}()
		}
		wg.Wait()

		if validateCtrl {
			if clusterGossips {
				network.validateControllerGossip(gossipingRouters, cb)
			} else {
				network.reportControllerGossipSkipped(cb)
			}
		}
	}

	return count, evalF, nil
}

func (network *Network) reportRouterGossipError(router *model.Router, err error, cb GossipValidationCallback) {
	cb(&mgmt_pb.GossipValidationDetails{
		ComponentType:   "router",
		ComponentId:     router.Id,
		ComponentName:   router.Name,
		ValidateSuccess: false,
		Message:         err.Error(),
	})
}

// reportControllerGossipSkipped records that this controller's own gossip store was not checked, for the
// same reason a router is skipped: with the store given up there is nothing for it to diverge from.
func (network *Network) reportControllerGossipSkipped(cb GossipValidationCallback) {
	pfxlog.Logger().Debug("skipping controller gossip validation, cluster cannot carry link gossip")
	cb(&mgmt_pb.GossipValidationDetails{
		ComponentType:   "controller",
		ComponentId:     network.GetAppId(),
		ComponentName:   network.GetAppId(),
		ValidateSuccess: true,
	})
}

// reportRouterGossipSkipped records a router the gossip check does not apply to, as a success with no link
// details.
//
// Reported rather than passed over so the count the caller waits on still adds up, and so the reason shows
// in the log rather than looking like a router that was silently not checked.
func (network *Network) reportRouterGossipSkipped(router *model.Router, reason string, cb GossipValidationCallback) {
	pfxlog.Logger().WithField("routerId", router.Id).WithField("reason", reason).
		Debug("skipping gossip validation for router")
	cb(&mgmt_pb.GossipValidationDetails{
		ComponentType:   "router",
		ComponentId:     router.Id,
		ComponentName:   router.Name,
		ValidateSuccess: true,
	})
}

// ValidateRouterGossip inspects a router for both its link registry ("links") and
// its own gossip store ("gossip-links"), then cross-checks both against this
// controller's gossip store.
//
// Reports whether the router publishes link gossip, which is false for a router that could not be
// inspected as well as for one too old to gossip.
func (network *Network) ValidateRouterGossip(router *model.Router, cb GossipValidationCallback) bool {
	request := &ctrl_pb.InspectRequest{RequestedValues: []string{"links", "gossip-links"}}
	resp := &ctrl_pb.InspectResponse{}
	respMsg, err := protobufs.MarshalTyped(request).WithTimeout(time.Minute).SendForReply(router.Control.GetDefaultSender())
	if err = protobufs.TypedResponse(resp).Unmarshall(respMsg, err); err != nil {
		network.reportRouterGossipError(router, err, cb)
		return false
	}

	var links *inspect.LinksInspectResult
	var routerGossip *inspect.RouterGossipLinksInspect
	for _, val := range resp.Values {
		switch val.Name {
		case "links":
			if err = json.Unmarshal([]byte(val.Value), &links); err != nil {
				network.reportRouterGossipError(router, err, cb)
				return false
			}
		case "gossip-links":
			if err = json.Unmarshal([]byte(val.Value), &routerGossip); err != nil {
				network.reportRouterGossipError(router, err, cb)
				return false
			}
		}
	}

	// A router too old to gossip does not know the value and simply omits it. It has nothing to be
	// inconsistent with, so it is skipped rather than reported as a router that failed to answer.
	if links != nil && routerGossip == nil {
		network.reportRouterGossipSkipped(router, "router does not publish link gossip", cb)
		return false
	}

	if links == nil || routerGossip == nil {
		msg := "router did not return both links and gossip-links"
		if len(resp.Errors) > 0 {
			msg = strings.Join(resp.Errors, ",")
		}
		network.reportRouterGossipError(router, errors.New(msg), cb)
		return false
	}

	// Index the router's own live gossip entries by gossip key.
	routerGossipByKey := map[string]*inspect.RouterGossipLinkEntry{}
	for i := range routerGossip.Entries {
		e := &routerGossip.Entries[i]
		if !e.Tombstone {
			routerGossipByKey[e.Key] = e
		}
	}

	result := &mgmt_pb.GossipValidationDetails{
		ComponentType:   "router",
		ComponentId:     router.Id,
		ComponentName:   router.Name,
		ValidateSuccess: true,
	}

	seen := map[string]bool{}

	// A router owns the gossip entries for the links it dialed. Each established
	// dialed link in the registry should have a matching live entry in both the
	// router's gossip store and this controller's gossip store.
	for _, link := range links.Links {
		if !link.Dialed {
			continue
		}
		key := LinkGossipKey(link.Id, link.Iteration)
		seen[key] = true

		detail := &mgmt_pb.GossipLinkDetail{
			LinkId:       link.Id,
			Iteration:    link.Iteration,
			DestRouterId: link.Dest,
			Dialed:       true,
			InSource:     true,
		}
		if rg, ok := routerGossipByKey[key]; ok {
			detail.InLocalGossip = true
			detail.GossipVersion = rg.Version
		}
		if _, ver, found := network.LinkGossipType.GetForOwner(router.Id, key); found {
			detail.InCtrlGossip = true
			if detail.GossipVersion == 0 {
				detail.GossipVersion = ver
			}
		}

		detail.IsValid = detail.InLocalGossip && detail.InCtrlGossip
		if !detail.InLocalGossip {
			detail.Messages = append(detail.Messages, "established dialed link missing from router gossip store (registry<->gossip desync)")
		} else if !detail.InCtrlGossip {
			detail.Messages = append(detail.Messages, "router gossip entry not present in controller gossip store (propagation gap)")
		}
		if !detail.IsValid {
			result.ValidateSuccess = false
		}
		result.LinkDetails = append(result.LinkDetails, detail)
	}

	// Router gossip entries with no backing established dialed link are stale.
	for key, rg := range routerGossipByKey {
		if seen[key] {
			continue
		}
		detail := &mgmt_pb.GossipLinkDetail{
			LinkId:        rg.LinkId,
			Iteration:     rg.Iteration,
			DestRouterId:  rg.DestRouter,
			Dialed:        true,
			InLocalGossip: true,
			GossipVersion: rg.Version,
		}
		if _, _, found := network.LinkGossipType.GetForOwner(router.Id, key); found {
			detail.InCtrlGossip = true
		}
		detail.Messages = append(detail.Messages, "router gossip entry has no backing established dialed link in registry (stale)")
		result.ValidateSuccess = false
		result.LinkDetails = append(result.LinkDetails, detail)
	}

	cb(result)
	return true
}

// validateControllerGossip diffs this controller's link manager against its gossip store: every link
// owned by a gossiping router should have a backing live gossip entry, and every live gossip entry
// should have a link in the manager.
//
// gossipingRouters is the set of routers that answered this pass saying they publish link gossip. Only
// their links are expected in the store: a router reporting links the pre-gossip way is deliberately
// kept out of it, and a router this controller could not reach has said nothing either way. That leaves
// links from routers connected elsewhere unchecked, which is the price of asking rather than assuming.
func (network *Network) validateControllerGossip(gossipingRouters map[string]struct{}, cb GossipValidationCallback) {
	result := &mgmt_pb.GossipValidationDetails{
		ComponentType:   "controller",
		ComponentId:     network.GetAppId(),
		ComponentName:   network.GetAppId(),
		ValidateSuccess: true,
	}

	type gossipEntry struct {
		iteration uint32
		version   uint64
		dest      string
	}
	gossipByLinkId := map[string]gossipEntry{}
	network.LinkGossipType.IterWithVersion(func(key string, value *ctrl_pb.RouterLinks_RouterLink, owner string, version uint64) {
		linkId, iteration := ParseLinkGossipKey(key)
		gossipByLinkId[linkId] = gossipEntry{iteration: iteration, version: version, dest: value.DestRouterId}
	})

	for _, link := range network.Link.GetLinkMap() {
		key := LinkGossipKey(link.Id, link.Iteration)
		owner := link.GetSrc().Id
		_, ver, found := network.LinkGossipType.GetForOwner(owner, key)
		_, ownerGossips := gossipingRouters[owner]
		detail := &mgmt_pb.GossipLinkDetail{
			LinkId:        link.Id,
			Iteration:     link.Iteration,
			DestRouterId:  link.DstId,
			InSource:      true,
			InLocalGossip: found,
			InCtrlGossip:  found,
			GossipVersion: ver,
			IsValid:       found || !ownerGossips,
		}
		if !found && ownerGossips {
			detail.Messages = append(detail.Messages, "link present in link manager but missing from gossip store (orphan)")
			result.ValidateSuccess = false
		}
		delete(gossipByLinkId, link.Id)
		result.LinkDetails = append(result.LinkDetails, detail)
	}

	for linkId, g := range gossipByLinkId {
		result.ValidateSuccess = false
		result.LinkDetails = append(result.LinkDetails, &mgmt_pb.GossipLinkDetail{
			LinkId:        linkId,
			Iteration:     g.iteration,
			DestRouterId:  g.dest,
			InLocalGossip: true,
			InCtrlGossip:  true,
			GossipVersion: g.version,
			IsValid:       false,
			Messages:      []string{"gossip entry has no link in link manager"},
		})
	}

	cb(result)
}
