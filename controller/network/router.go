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
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/fields"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/openziti/foundation/v2/versions"
	"google.golang.org/protobuf/proto"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/storage/boltz"
	cmap "github.com/orcaman/concurrent-map/v2"
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
	ConnectTime time.Time
	VersionInfo *versions.VersionInfo
	routerLinks RouterLinks
	Cost        uint16
	NoTraversal bool
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

type RouterManager struct {
	baseEntityManager[*Router]
	cache     cmap.ConcurrentMap[*Router]
	connected cmap.ConcurrentMap[*Router]
	store     db.RouterStore
}

func newRouterManager(managers *Managers) *RouterManager {
	result := &RouterManager{
		baseEntityManager: newBaseEntityManager(managers, managers.stores.Router, func() *Router {
			return &Router{}
		}),
		cache:     cmap.New[*Router](),
		connected: cmap.New[*Router](),
		store:     managers.stores.Router,
	}
	result.populateEntity = result.populateRouter

	managers.stores.Router.AddListener(boltz.EventUpdate, func(i ...interface{}) {
		for _, val := range i {
			if router, ok := val.(*db.Router); ok {
				result.UpdateCachedRouter(router.Id)
			} else {
				pfxlog.Logger().Errorf("error in router listener. expected *db.Router, got %T", val)
			}
		}
	})

	managers.stores.Router.AddListener(boltz.EventDelete, func(i ...interface{}) {
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

func (self *RouterManager) markConnected(r *Router) {
	if router, _ := self.connected.Get(r.Id); router != nil {
		if ch := router.Control; ch != nil {
			if err := ch.Close(); err != nil {
				pfxlog.Logger().WithError(err).Error("error closing control channel")
			}
		}
	}

	r.Connected.Set(true)
	self.connected.Set(r.Id, r)
}

func (self *RouterManager) markDisconnected(r *Router) {
	r.Connected.Set(false)
	self.connected.RemoveCb(r.Id, func(key string, v *Router, exists bool) bool {
		if exists && v != r {
			pfxlog.Logger().WithField("routerId", r.Id).Info("router not current connect, not clearing from connected map")
			return false
		}
		return exists
	})
	r.routerLinks.Clear()
}

func (self *RouterManager) IsConnected(id string) bool {
	return self.connected.Has(id)
}

func (self *RouterManager) getConnected(id string) *Router {
	if router, found := self.connected.Get(id); found {
		return router
	}
	return nil
}

func (self *RouterManager) allConnected() []*Router {
	var routers []*Router
	for v := range self.connected.IterBuffered() {
		routers = append(routers, v.Val)
	}
	return routers
}

func (self *RouterManager) connectedCount() int {
	return self.connected.Count()
}

func (self *RouterManager) Create(entity *Router) error {
	return DispatchCreate[*Router](self, entity)
}

func (self *RouterManager) ApplyCreate(cmd *command.CreateEntityCommand[*Router]) error {
	router := cmd.Entity
	err := self.db.Update(func(tx *bbolt.Tx) error {
		return self.store.Create(boltz.NewMutateContext(tx), router.toBolt())
	})
	if err != nil {
		self.cache.Set(router.Id, router)
	}
	return err
}

func (self *RouterManager) Read(id string) (entity *Router, err error) {
	err = self.db.View(func(tx *bbolt.Tx) error {
		entity, err = self.readInTx(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return entity, err
}

func (self *RouterManager) readUncached(id string) (*Router, error) {
	entity := &Router{}
	err := self.db.View(func(tx *bbolt.Tx) error {
		return self.readEntityInTx(tx, id, entity)
	})
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (self *RouterManager) readInTx(tx *bbolt.Tx, id string) (*Router, error) {
	if router, found := self.cache.Get(id); found {
		return router, nil
	}

	entity := &Router{}
	if err := self.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}

	self.cache.Set(id, entity)
	return entity, nil
}

func (self *RouterManager) populateRouter(entity *Router, _ *bbolt.Tx, boltEntity boltz.Entity) error {
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

func (self *RouterManager) Update(entity *Router, updatedFields fields.UpdatedFields) error {
	return DispatchUpdate[*Router](self, entity, updatedFields)
}

func (self *RouterManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Router]) error {
	return self.updateGeneral(cmd.Entity, cmd.UpdatedFields)
}

func (self *RouterManager) HandleRouterDelete(id string) {
	log := pfxlog.Logger().WithField("routerId", id)
	log.Debug("processing router delete")
	self.cache.Remove(id)

	// if we close the control channel, the router will get removed from the connected cache. We don't do it
	// here because it results in deadlock
	if router, found := self.connected.Get(id); found {
		if ctrl := router.Control; ctrl != nil {
			_ = ctrl.Close()
			log.Warn("connected router deleted, disconnecting router")
		} else {
			log.Warn("deleted router in connected cache doesn't have a connected control channel")
		}
	} else {
		log.Debug("deleted router not connected, no further action required")
	}

	self.network.routerDeleted(id)
}

func (self *RouterManager) UpdateCachedRouter(id string) {
	log := pfxlog.Logger().WithField("routerId", id)
	if router, err := self.readUncached(id); err != nil {
		log.WithError(err).Error("failed to read router for cache update")
	} else {
		updateCb := func(key string, v *Router, exist bool) bool {
			if !exist {
				return false
			}

			v.Name = router.Name
			v.Fingerprint = router.Fingerprint
			v.Cost = router.Cost
			v.NoTraversal = router.NoTraversal

			return false
		}

		self.cache.RemoveCb(id, updateCb)
		self.connected.RemoveCb(id, updateCb)
	}
}

func (self *RouterManager) RemoveFromCache(id string) {
	self.cache.Remove(id)
}

func (self *RouterManager) Marshall(entity *Router) ([]byte, error) {
	tags, err := cmd_pb.EncodeTags(entity.Tags)
	if err != nil {
		return nil, err
	}

	var fingerprint []byte
	if entity.Fingerprint != nil {
		fingerprint = []byte(*entity.Fingerprint)
	}

	msg := &cmd_pb.Router{
		Id:          entity.Id,
		Name:        entity.Name,
		Fingerprint: fingerprint,
		Cost:        uint32(entity.Cost),
		NoTraversal: entity.NoTraversal,
		Tags:        tags,
	}

	return proto.Marshal(msg)
}

func (self *RouterManager) Unmarshall(bytes []byte) (*Router, error) {
	msg := &cmd_pb.Router{}
	if err := proto.Unmarshal(bytes, msg); err != nil {
		return nil, err
	}

	var fingerprint *string
	if msg.Fingerprint != nil {
		tmp := string(msg.Fingerprint)
		fingerprint = &tmp
	}

	return &Router{
		BaseEntity: models.BaseEntity{
			Id:   msg.Id,
			Tags: cmd_pb.DecodeTags(msg.Tags),
		},
		Name:        msg.Name,
		Fingerprint: fingerprint,
		Cost:        uint16(msg.Cost),
		NoTraversal: msg.NoTraversal,
	}, nil
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
