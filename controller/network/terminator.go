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
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
)

type Terminator struct {
	models.BaseEntity
	Service  string
	Router   string
	Binding  string
	Address  string
	Cost     uint16
	PeerData map[uint32][]byte
}

func (entity *Terminator) GetServiceId() string {
	return entity.Service
}

func (entity *Terminator) GetRouterId() string {
	return entity.Router
}

func (entity *Terminator) GetBinding() string {
	return entity.Binding
}

func (entity *Terminator) GetAddress() string {
	return entity.Address
}

func (entity *Terminator) GetCost() uint16 {
	return entity.Cost
}

func (entity *Terminator) GetPeerData() map[uint32][]byte {
	return entity.PeerData
}

func (entity *Terminator) fillFrom(_ Controller, _ *bbolt.Tx, boltEntity boltz.Entity) error {
	boltTerminator, ok := boltEntity.(*db.Terminator)
	if !ok {
		return errors.Errorf("unexpected type %v when filling model terminator", reflect.TypeOf(boltEntity))
	}
	entity.Service = boltTerminator.Service
	entity.Router = boltTerminator.Router
	entity.Binding = boltTerminator.Binding
	entity.Address = boltTerminator.Address
	entity.PeerData = boltTerminator.PeerData
	entity.Cost = boltTerminator.Cost
	entity.FillCommon(boltTerminator)
	return nil
}

func (entity *Terminator) toBolt() *db.Terminator {
	return &db.Terminator{
		BaseExtEntity: *boltz.NewExtEntity(entity.Id, entity.Tags),
		Service:       entity.Service,
		Router:        entity.Router,
		Binding:       entity.Binding,
		Address:       entity.Address,
		Cost:          entity.Cost,
		PeerData:      entity.PeerData,
	}
}

func newTerminatorController(controllers *Controllers) *TerminatorController {
	result := &TerminatorController{
		baseController: newController(controllers, controllers.stores.Terminator),
		store:          controllers.stores.Terminator,
	}
	result.impl = result

	controllers.stores.Terminator.On(boltz.EventCreate, func(params ...interface{}) {
		for _, entity := range params {
			if terminator, ok := entity.(*db.Terminator); ok {
				xt.GlobalCosts().TerminatorCreated(terminator.Id)
			}
		}
	})

	controllers.stores.Terminator.On(boltz.EventDelete, func(params ...interface{}) {
		for _, entity := range params {
			if terminator, ok := entity.(*db.Terminator); ok {
				xt.GlobalCosts().ClearCost(terminator.Id)
			}
		}
	})

	return result
}

type TerminatorController struct {
	baseController
	store db.TerminatorStore
}

func (ctrl *TerminatorController) newModelEntity() boltEntitySink {
	return &Terminator{}
}

func (ctrl *TerminatorController) Create(s *Terminator) (string, error) {
	var id string
	var err error
	err = ctrl.db.Update(func(tx *bbolt.Tx) error {
		id, err = ctrl.createInTx(boltz.NewMutateContext(tx), s)
		return err
	})
	return id, err
}

func (ctrl *TerminatorController) checkBinding(terminator *Terminator) {
	if terminator.Binding == "" {
		if strings.HasPrefix(terminator.Address, "udp:") {
			terminator.Binding = "udp"
		} else {
			terminator.Binding = "transport"
		}
	}
}

func (ctrl *TerminatorController) createInTx(ctx boltz.MutateContext, terminator *Terminator) (string, error) {
	ctrl.checkBinding(terminator)
	boltTerminator := terminator.toBolt()
	if err := ctrl.GetStore().Create(ctx, boltTerminator); err != nil {
		return "", err
	}
	return boltTerminator.Id, nil
}

func (ctrl *TerminatorController) Update(terminator *Terminator) error {
	return ctrl.db.Update(func(tx *bbolt.Tx) error {
		ctrl.checkBinding(terminator)
		return ctrl.GetStore().Update(boltz.NewMutateContext(tx), terminator.toBolt(), nil)
	})
}

func (ctrl *TerminatorController) Patch(terminator *Terminator, checker boltz.FieldChecker) error {
	return ctrl.db.Update(func(tx *bbolt.Tx) error {
		ctrl.checkBinding(terminator)
		return ctrl.GetStore().Update(boltz.NewMutateContext(tx), terminator.toBolt(), checker)
	})
}

func (ctrl *TerminatorController) Read(id string) (entity *Terminator, err error) {
	err = ctrl.db.View(func(tx *bbolt.Tx) error {
		entity, err = ctrl.readInTx(tx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return entity, err
}

func (ctrl *TerminatorController) readInTx(tx *bbolt.Tx, id string) (*Terminator, error) {
	entity := &Terminator{}
	err := ctrl.readEntityInTx(tx, id, entity)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (ctrl *TerminatorController) Delete(id string) error {
	return ctrl.db.Update(func(tx *bbolt.Tx) error {
		return ctrl.store.DeleteById(boltz.NewMutateContext(tx), id)
	})
}

func (ctrl *TerminatorController) Query(query string) (*TerminatorListResult, error) {
	result := &TerminatorListResult{controller: ctrl}
	if err := ctrl.list(query, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

type TerminatorListResult struct {
	controller *TerminatorController
	Entities   []*Terminator
	models.QueryMetaData
}

func (result *TerminatorListResult) collect(tx *bbolt.Tx, ids []string, qmd *models.QueryMetaData) error {
	result.QueryMetaData = *qmd
	for _, id := range ids {
		terminator, err := result.controller.readInTx(tx, id)
		if err != nil {
			return err
		}
		result.Entities = append(result.Entities, terminator)
	}
	return nil
}

type RoutingTerminator struct {
	RouteCost uint32
	Stats     xt.Stats
	*Terminator
}

func (r *RoutingTerminator) GetPrecedence() xt.Precedence {
	return r.Stats.GetPrecedence()
}

func (r *RoutingTerminator) GetTerminatorStats() xt.Stats {
	return r.Stats
}

func (r *RoutingTerminator) GetRouteCost() uint32 {
	return r.RouteCost
}
