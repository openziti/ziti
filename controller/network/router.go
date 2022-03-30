/*
	Copyright NetFoundry, Inc.

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
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/common"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/foundation/util/concurrenz"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

type Listener interface {
	AdvertiseAddress() string
	Protocol() string
}

type Router struct {
	models.BaseEntity
	Name        string
	Fingerprint *string
	Listeners   []Listener
	Control     channel.Channel
	Connected   concurrenz.AtomicBoolean
	VersionInfo *common.VersionInfo
	routerLinks RouterLinks
	Cost        uint16
	NoTraversal bool
}

func (entity *Router) fillFrom(_ Controller, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltRouter, ok := boltEntity.(*db.Router)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model router", reflect.TypeOf(boltEntity))
	}
	entity.Name = boltRouter.Name
	entity.Fingerprint = boltRouter.Fingerprint
	entity.Cost = boltRouter.Cost
	entity.NoTraversal = boltRouter.NoTraversal
	entity.FillCommon(boltRouter)
	return nil
}

func (entity *Router) toBolt() boltz.Entity {
	return &db.Router{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
		Fingerprint:   entity.Fingerprint,
		Cost:          entity.Cost,
		NoTraversal:   entity.NoTraversal,
	}
}

func (entity *Router) AddLinkListener(addr, linkProtocol string, linkCostTags []string) {
	entity.Listeners = append(entity.Listeners, linkListener{
		addr:         addr,
		linkProtocol: linkProtocol,
		linkCostTags: linkCostTags,
	})
}

func NewRouter(id, name, fingerprint string, cost uint16, noTraversal bool) *Router {
	if name == "" {
		name = id
	}
	result := &Router{
		BaseEntity:  models.BaseEntity{Id: id},
		Name:        name,
		Fingerprint: &fingerprint,
		Cost:        cost,
		NoTraversal: noTraversal,
	}
	result.routerLinks.allLinks.Store([]*Link{})
	result.routerLinks.linkByRouter.Store(map[string][]*Link{})
	return result
}

type RouterController struct {
	baseController
	cache     cmap.ConcurrentMap
	connected cmap.ConcurrentMap
	store     db.RouterStore
}

func (ctrl *RouterController) newModelEntity() boltEntitySink {
	return &Router{}
}

func newRouterController(controllers *Controllers) *RouterController {
	result := &RouterController{
		baseController: newController(controllers, controllers.stores.Router),
		cache:          cmap.New(),
		connected:      cmap.New(),
		store:          controllers.stores.Router,
	}
	result.impl = result

	controllers.stores.Router.AddListener(boltz.EventUpdate, func(i ...interface{}) {
		for _, val := range i {
			if router, ok := val.(*db.Router); ok {
				result.UpdateCachedRouter(router.Id)
			} else {
				pfxlog.Logger().Errorf("error in router listener. expected *db.Router, got %T", val)
			}
		}
	})

	controllers.stores.Router.AddListener(boltz.EventDelete, func(i ...interface{}) {
		for _, val := range i {
			if router, ok := val.(*db.Router); ok {
				result.HandleRouterDelete(router.Id)
			} else {
				pfxlog.Logger().Errorf("error in router listener. expected *db.Router, got %T", val)
			}
		}
	})

	return result
}

func (ctrl *RouterController) markConnected(r *Router) {
	r.Connected.Set(true)
	ctrl.connected.Set(r.Id, r)
}

func (ctrl *RouterController) markDisconnected(r *Router) {
	r.Connected.Set(false)
	ctrl.connected.Remove(r.Id)
	r.routerLinks.Clear()
}

func (ctrl *RouterController) IsConnected(id string) bool {
	return ctrl.connected.Has(id)
}

func (ctrl *RouterController) getConnected(id string) *Router {
	if t, found := ctrl.connected.Get(id); found {
		return t.(*Router)
	}
	return nil
}

func (ctrl *RouterController) allConnected() []*Router {
	var routers []*Router
	for v := range ctrl.connected.IterBuffered() {
		routers = append(routers, v.Val.(*Router))
	}
	return routers
}

func (ctrl *RouterController) connectedCount() int {
	return ctrl.connected.Count()
}

func (ctrl *RouterController) Create(router *Router) error {
	err := ctrl.db.Update(func(tx *bbolt.Tx) error {
		return ctrl.store.Create(boltz.NewMutateContext(tx), router.toBolt())
	})
	if err != nil {
		ctrl.cache.Set(router.Id, router)
	}
	return err
}

func (ctrl *RouterController) Delete(id string) error {
	err := ctrl.db.Update(func(tx *bbolt.Tx) error {
		return ctrl.store.DeleteById(boltz.NewMutateContext(tx), id)
	})
	if err == nil {
		ctrl.cache.Remove(id)
	}
	return err
}

func (ctrl *RouterController) Read(id string) (entity *Router, err error) {
	err = ctrl.db.View(func(tx *bbolt.Tx) error {
		entity, err = ctrl.readInTx(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return entity, err
}

func (ctrl *RouterController) readUncached(id string) (*Router, error) {
	entity := &Router{}
	err := ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.readEntityInTx(tx, id, entity)
	})
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (ctrl *RouterController) readInTx(tx *bbolt.Tx, id string) (*Router, error) {
	if t, found := ctrl.cache.Get(id); found {
		return t.(*Router), nil
	}

	entity := &Router{}
	if err := ctrl.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}

	ctrl.cache.Set(id, entity)
	return entity, nil
}

func (ctrl *RouterController) Update(r *Router) error {
	if err := ctrl.updateGeneral(r, nil); err != nil {
		return err
	}

	return nil
}

func (ctrl *RouterController) Patch(r *Router, checker boltz.FieldChecker) error {
	if err := ctrl.updateGeneral(r, checker); err != nil {
		return err
	}

	return nil
}

func (ctrl *RouterController) HandleRouterDelete(id string) {
	log := pfxlog.Logger().WithField("routerId", id)
	log.Debug("processing router delete")
	ctrl.cache.Remove(id)

	// if we close the control channel, the router will get removed from the connected cache. We don't do it
	// here because it results in deadlock
	if v, found := ctrl.connected.Get(id); found {
		if router, ok := v.(*Router); ok {
			if ctrl := router.Control; ctrl != nil {
				_ = ctrl.Close()
				log.Warn("connected router deleted, disconnecting router")
			} else {
				log.Warn("deleted router in connected cache doesn't have a connected control channel")
			}
		} else {
			log.Errorf("cached router of wrong type, expected %T, was %T", &Router{}, v)
		}
	} else {
		log.Debug("deleted router not connected, no further action required")
	}
}

func (ctrl *RouterController) UpdateCachedRouter(id string) {
	log := pfxlog.Logger().WithField("routerId", id)
	if router, err := ctrl.readUncached(id); err != nil {
		log.WithError(err).Error("failed to read router for cache update")
	} else {
		updateCb := func(key string, v interface{}, exist bool) bool {
			if !exist {
				return false
			}

			if cached, ok := v.(*Router); ok {
				cached.Name = router.Name
				cached.Fingerprint = router.Fingerprint
				cached.Cost = router.Cost
				cached.NoTraversal = router.NoTraversal
			} else {
				log.Errorf("cached router of wrong type, expected %T, was %T", &Router{}, v)
			}

			return false
		}

		ctrl.cache.RemoveCb(id, updateCb)
		ctrl.connected.RemoveCb(id, updateCb)
	}
}

type RouterLinks struct {
	sync.Mutex
	allLinks     atomic.Value
	linkByRouter atomic.Value
}

func (self *RouterLinks) GetLinks() []*Link {
	result := self.allLinks.Load()
	if result == nil {
		return nil
	}
	return result.([]*Link)
}

func (self *RouterLinks) GetLinksByRouter() map[string][]*Link {
	result := self.linkByRouter.Load()
	if result == nil {
		return nil
	}
	return result.(map[string][]*Link)
}

func (self *RouterLinks) Add(link *Link, other *Router) {
	self.Lock()
	defer self.Unlock()
	links := self.GetLinks()
	newLinks := make([]*Link, 0, len(links)+1)
	for _, l := range links {
		newLinks = append(newLinks, l)
	}
	newLinks = append(newLinks, link)
	self.allLinks.Store(newLinks)

	byRouter := self.GetLinksByRouter()
	newLinksByRouter := map[string][]*Link{}
	for k, v := range byRouter {
		newLinksByRouter[k] = v
	}
	forRouterList := newLinksByRouter[other.Id]
	newForRouterList := append([]*Link{link}, forRouterList...)
	newLinksByRouter[other.Id] = newForRouterList
	self.linkByRouter.Store(newLinksByRouter)
}

func (self *RouterLinks) Remove(link *Link, other *Router) {
	self.Lock()
	defer self.Unlock()
	links := self.GetLinks()
	newLinks := make([]*Link, 0, len(links)+1)
	for _, l := range links {
		if link != l {
			newLinks = append(newLinks, l)
		}
	}
	self.allLinks.Store(newLinks)

	byRouter := self.GetLinksByRouter()
	newLinksByRouter := map[string][]*Link{}
	for k, v := range byRouter {
		newLinksByRouter[k] = v
	}
	forRouterList := newLinksByRouter[other.Id]
	var newForRouterList []*Link
	for _, l := range forRouterList {
		if l != link {
			newForRouterList = append(newForRouterList, l)
		}
	}
	if len(newForRouterList) == 0 {
		delete(newLinksByRouter, other.Id)
	} else {
		newLinksByRouter[other.Id] = newForRouterList
	}

	self.linkByRouter.Store(newLinksByRouter)

}

func (self *RouterLinks) Clear() {
	self.allLinks.Store([]*Link{})
	self.linkByRouter.Store(map[string][]*Link{})
}

type linkListener struct {
	addr         string
	linkProtocol string
	linkCostTags []string
}

func (self linkListener) AdvertiseAddress() string {
	return self.addr
}

func (self linkListener) Protocol() string {
	return self.linkProtocol
}
