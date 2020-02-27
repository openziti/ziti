/*
	Copyright 2019 NetFoundry, Inc.

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
	"fmt"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/orcaman/concurrent-map"
	"go.etcd.io/bbolt"
)

type Router struct {
	Id                 string
	Fingerprint        string
	AdvertisedListener transport.Address
	Control            channel2.Channel
	CostFactor         int
}

func (entity *Router) toBolt() *db.Router {
	return &db.Router{
		Id:          entity.Id,
		Fingerprint: entity.Fingerprint,
	}
}

func NewRouter(id, fingerprint string) *Router {
	return &Router{
		Id:          id,
		Fingerprint: fingerprint,
	}
}

func newRouter(id string, fingerprint string, advLstnr transport.Address, ctrl channel2.Channel) *Router {
	return &Router{
		Id:                 id,
		Fingerprint:        fingerprint,
		AdvertisedListener: advLstnr,
		Control:            ctrl,
		CostFactor:         1,
	}
}

type routerController struct {
	connected cmap.ConcurrentMap // map[string]*Router
	db        *db.Db
	stores    *db.Stores
	store     db.RouterStore
}

func newRouterController(db *db.Db, stores *db.Stores) *routerController {
	return &routerController{
		connected: cmap.New(),
		db:        db,
		stores:    stores,
		store:     stores.Router,
	}
}

func (c *routerController) markConnected(r *Router) {
	c.connected.Set(r.Id, r)
}

func (c *routerController) markDisconnected(r *Router) {
	c.connected.Remove(r.Id)
}

func (c *routerController) isConnected(id string) bool {
	return c.connected.Has(id)
}

func (c *routerController) getConnected(id string) (*Router, bool) {
	if t, found := c.connected.Get(id); found {
		return t.(*Router), true
	}
	return nil, false
}

func (c *routerController) allConnected() []*Router {
	routers := make([]*Router, 0)
	for i := range c.connected.IterBuffered() {
		routers = append(routers, i.Val.(*Router))
	}
	return routers
}

func (c *routerController) connectedCount() int {
	return c.connected.Count()
}

func (c *routerController) create(router *Router) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		return c.store.Create(boltz.NewMutateContext(tx), router.toBolt())
	})
}

func (c *routerController) update(router *Router) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		return c.store.Update(boltz.NewMutateContext(tx), router.toBolt(), nil)
	})
}

func (c *routerController) remove(id string) error {
	return c.db.Update(func(tx *bbolt.Tx) error {
		return c.store.DeleteById(boltz.NewMutateContext(tx), id)
	})
}

func (c *routerController) loadOneById(id string) (*Router, error) {
	var router *Router
	err := c.db.View(func(tx *bbolt.Tx) error {
		boltRouter, err := c.store.LoadOneById(tx, id)
		if err != nil {
			return err
		}
		if boltRouter == nil {
			return fmt.Errorf("missing router '%s'", id)
		}
		router = c.fromBolt(boltRouter)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return router, err
}

func (c *routerController) all() ([]*Router, error) {
	var routers []*Router
	err := c.db.View(func(tx *bbolt.Tx) error {
		ids, _, err := c.store.QueryIds(tx, "true")
		if err != nil {
			return err
		}
		for _, id := range ids {
			router, err := c.store.LoadOneById(tx, id)
			if err != nil {
				return err
			}
			routers = append(routers, c.fromBolt(router))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return routers, nil
}

func (c *routerController) fromBolt(entity *db.Router) *Router {
	return &Router{
		Id:          entity.Id,
		Fingerprint: entity.Fingerprint,
	}
}
