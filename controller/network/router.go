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
)

type Router struct {
	models.BaseEntity
	Name               string
	Fingerprint        *string
	AdvertisedListener string
	Control            channel2.Channel
	Connected          concurrenz.AtomicBoolean
	VersionInfo        *common.VersionInfo
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
	return &Router{
		BaseEntity:  models.BaseEntity{Id: id},
		Name:        name,
		Fingerprint: &fingerprint,
	}
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
	for i := range ctrl.connected.IterBuffered() {
		routers = append(routers, i.Val.(*Router))
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
