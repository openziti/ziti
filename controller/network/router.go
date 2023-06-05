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
	"github.com/openziti/fabric/controller/change"
	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/fields"
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/openziti/foundation/v2/versions"
	"google.golang.org/protobuf/proto"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/storage/boltz"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

const (
	RouterQuiesceFlag   uint32 = 1
	RouterDequiesceFlag uint32 = 2
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
	Connected   atomic.Bool
	ConnectTime time.Time
	VersionInfo *versions.VersionInfo
	routerLinks RouterLinks
	Cost        uint16
	NoTraversal bool
	Disabled    bool
}

func (entity *Router) toBolt() *db.Router {
	return &db.Router{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Name:          entity.Name,
		Fingerprint:   entity.Fingerprint,
		Cost:          entity.Cost,
		NoTraversal:   entity.NoTraversal,
		Disabled:      entity.Disabled,
	}
}

func (entity *Router) AddLinkListener(addr, linkProtocol string, linkCostTags []string, groups []string) {
	entity.Listeners = append(entity.Listeners, linkListener{
		addr:         addr,
		linkProtocol: linkProtocol,
		linkCostTags: linkCostTags,
		groups:       groups,
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
	baseEntityManager[*Router, *db.Router]
	cache     cmap.ConcurrentMap[string, *Router]
	connected cmap.ConcurrentMap[string, *Router]
	store     db.RouterStore
}

func newRouterManager(managers *Managers) *RouterManager {
	result := &RouterManager{
		baseEntityManager: newBaseEntityManager[*Router, *db.Router](managers, managers.stores.Router, func() *Router {
			return &Router{}
		}),
		cache:     cmap.New[*Router](),
		connected: cmap.New[*Router](),
		store:     managers.stores.Router,
	}
	result.populateEntity = result.populateRouter

	managers.stores.Router.AddEntityIdListener(result.UpdateCachedRouter, boltz.EntityUpdated)
	managers.stores.Router.AddEntityIdListener(result.HandleRouterDelete, boltz.EntityDeleted)

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

	r.Connected.Store(true)
	self.connected.Set(r.Id, r)
}

func (self *RouterManager) markDisconnected(r *Router) {
	r.Connected.Store(false)
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

func (self *RouterManager) Create(entity *Router, ctx *change.Context) error {
	return DispatchCreate[*Router](self, entity, ctx)
}

func (self *RouterManager) ApplyCreate(cmd *command.CreateEntityCommand[*Router], ctx boltz.MutateContext) error {
	router := cmd.Entity
	err := self.db.Update(ctx, func(ctx boltz.MutateContext) error {
		return self.store.Create(ctx, router.toBolt())
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
	entity.Disabled = boltRouter.Disabled
	entity.FillCommon(boltRouter)
	return nil
}

func (self *RouterManager) Update(entity *Router, updatedFields fields.UpdatedFields, ctx *change.Context) error {
	return DispatchUpdate[*Router](self, entity, updatedFields, ctx)
}

func (self *RouterManager) ApplyUpdate(cmd *command.UpdateEntityCommand[*Router], ctx boltz.MutateContext) error {
	if cmd.Flags == RouterQuiesceFlag {
		return self.ApplyQuiesce(cmd, ctx)
	} else if cmd.Flags == RouterDequiesceFlag {
		return self.ApplyDequiesce(cmd, ctx)
	}

	return self.updateGeneral(ctx, cmd.Entity, cmd.UpdatedFields)
}

// QuiesceRouter marks all terminators on the router as failed, so that new traffic will avoid this router, if there's
// any alternative path
func (self *RouterManager) QuiesceRouter(entity *Router, ctx *change.Context) error {
	cmd := &command.UpdateEntityCommand[*Router]{
		Context:       ctx,
		Updater:       self,
		Entity:        entity,
		UpdatedFields: nil,
		Flags:         RouterQuiesceFlag,
	}

	return self.Dispatch(cmd)
}

// DequiesceRouter returns all routers with a saved precedence that are in a failed state back to their saved state
func (self *RouterManager) DequiesceRouter(entity *Router, ctx *change.Context) error {
	cmd := &command.UpdateEntityCommand[*Router]{
		Context:       ctx,
		Updater:       self,
		Entity:        entity,
		UpdatedFields: nil,
		Flags:         RouterDequiesceFlag,
	}

	return self.Dispatch(cmd)
}

func (self *RouterManager) ApplyQuiesce(cmd *command.UpdateEntityCommand[*Router], ctx boltz.MutateContext) error {
	return self.UpdateTerminators(cmd.Entity, ctx, func(terminator *db.Terminator) error {
		if terminator.Precedence == xt.Precedences.Failed.String() {
			return nil
		}

		currentPrecedence := terminator.Precedence
		terminator.SavedPrecedence = &currentPrecedence
		terminator.Precedence = xt.Precedences.Failed.String()

		return self.Terminators.store.Update(ctx.GetSystemContext(), terminator, boltz.MapFieldChecker{
			db.FieldTerminatorPrecedence:      struct{}{},
			db.FieldTerminatorSavedPrecedence: struct{}{},
		})
	})
}

func (self *RouterManager) ApplyDequiesce(cmd *command.UpdateEntityCommand[*Router], ctx boltz.MutateContext) error {
	return self.UpdateTerminators(cmd.Entity, ctx, func(terminator *db.Terminator) error {
		if terminator.SavedPrecedence == nil || terminator.Precedence != xt.Precedences.Failed.String() {
			return nil
		}

		terminator.Precedence = *terminator.SavedPrecedence
		terminator.SavedPrecedence = nil

		return self.Terminators.store.Update(ctx.GetSystemContext(), terminator, boltz.MapFieldChecker{
			db.FieldTerminatorPrecedence:      struct{}{},
			db.FieldTerminatorSavedPrecedence: struct{}{},
		})
	})
}

func (self *RouterManager) UpdateTerminators(router *Router, ctx boltz.MutateContext, f func(terminator *db.Terminator) error) error {
	return self.db.Update(ctx, func(ctx boltz.MutateContext) error {
		terminatorIds := self.store.GetRelatedEntitiesIdList(ctx.Tx(), router.Id, db.EntityTypeTerminators)
		for _, terminatorId := range terminatorIds {
			terminator, _, err := self.Terminators.store.FindById(ctx.Tx(), terminatorId)
			if err != nil {
				return err
			}
			if err = f(terminator); err != nil {
				return err
			}
		}
		return nil
	})
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
			v.Disabled = router.Disabled

			if v.Disabled {
				if ctrl := v.Control; ctrl != nil {
					_ = ctrl.Close()
					log.Warn("connected router disabled, disconnecting router")
				}
			}

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
		Disabled:    entity.Disabled,
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
		Disabled:    msg.Disabled,
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
	newLinks = append(newLinks, links...)
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
	groups       []string
}

func (self linkListener) AdvertiseAddress() string {
	return self.addr
}

func (self linkListener) Protocol() string {
	return self.linkProtocol
}

func (self linkListener) Groups() []string {
	return self.groups
}
