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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/common"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"sync"
	"sync/atomic"
)

type Router struct {
	models.BaseEntity
	Name               string
	Fingerprint        *string
	AdvertisedListener string
	Control            channel2.Channel
	Connected          concurrenz.AtomicBoolean
	VersionInfo        *common.VersionInfo
	routerLinks        RouterLinks
}

func (entity *Router) fillFrom(_ Controller, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltRouter, ok := boltEntity.(*db.Router)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model router", reflect.TypeOf(boltEntity))
	}
	entity.Name = boltRouter.Name
	entity.Fingerprint = boltRouter.Fingerprint
	entity.FillCommon(boltRouter)
	return nil
}

func (entity *Router) toBolt() *db.Router {
	return &db.Router{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
		Fingerprint:   entity.Fingerprint,
	}
}

func NewRouter(id, name, fingerprint string) *Router {
	if name == "" {
		name = id
	}
	result := &Router{
		BaseEntity:  models.BaseEntity{Id: id},
		Name:        name,
		Fingerprint: &fingerprint,
	}
	result.routerLinks.Store([]*Link{})
	return result
}

type routerCopyOnWriteMap struct {
	sync.Mutex
	atomic.Value
}

func newRouterCopyOnWriteMap() *routerCopyOnWriteMap {
	result := &routerCopyOnWriteMap{}
	result.Store(map[string]*Router{})
	return result
}

func (self *routerCopyOnWriteMap) Has(id string) bool {
	_, found := self.getCurrentMap()[id]
	return found
}

func (self *routerCopyOnWriteMap) Count() int {
	return len(self.getCurrentMap())
}

func (self *routerCopyOnWriteMap) Get(id string) (*Router, bool) {
	r, found := self.getCurrentMap()[id]
	return r, found
}

func (self *routerCopyOnWriteMap) Set(id string, router *Router) {
	self.Lock()
	defer self.Unlock()
	m := self.getMapCopy()
	m[id] = router
	self.Store(m)
}

func (self *routerCopyOnWriteMap) Remove(id string) {
	self.Lock()
	defer self.Unlock()
	m := self.getMapCopy()
	delete(m, id)
	self.Store(m)
}

func (self *routerCopyOnWriteMap) getCurrentMap() map[string]*Router {
	return self.Load().(map[string]*Router)
}

func (self *routerCopyOnWriteMap) getMapCopy() map[string]*Router {
	m := self.getCurrentMap()
	mapCopy := map[string]*Router{}
	for k, v := range m {
		mapCopy[k] = v
	}
	return mapCopy
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

func (ctrl *RouterController) UpdateCachedFingerprint(id, fingerprint string) {
	if val, ok := ctrl.cache.Get(id); ok {
		if router, ok := val.(*Router); ok {
			router.Fingerprint = &fingerprint
		} else {
			pfxlog.Logger().Errorf("encountered %t in router cache, expected *Router", val)
		}
	}
}

type RouterLinks struct {
	sync.Mutex
	atomic.Value
}

func (self *RouterLinks) GetLinks() []*Link {
	result := self.Load()
	if result == nil {
		return nil
	}
	return result.([]*Link)
}

func (self *RouterLinks) Add(link *Link) {
	self.Lock()
	defer self.Unlock()
	links := self.GetLinks()
	newLinks := make([]*Link, 0, len(links)+1)
	for _, l := range links {
		newLinks = append(newLinks, l)
	}
	newLinks = append(newLinks, link)
	self.Store(newLinks)
}

func (self *RouterLinks) Remove(link *Link) {
	self.Lock()
	defer self.Unlock()
	links := self.GetLinks()
	newLinks := make([]*Link, 0, len(links)+1)
	for _, l := range links {
		if link != l {
			newLinks = append(newLinks, l)
		}
	}
	self.Store(newLinks)
}

func (self *RouterLinks) Clear() {
	self.Store([]*Link{})
}
