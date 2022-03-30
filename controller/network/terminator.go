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
	"github.com/openziti/fabric/controller/xt"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
)

type Terminator struct {
	models.BaseEntity
	Service        string
	Router         string
	Binding        string
	Address        string
	Identity       string
	IdentitySecret []byte
	Cost           uint16
	Precedence     xt.Precedence
	PeerData       map[uint32][]byte
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

func (entity *Terminator) GetIdentity() string {
	return entity.Identity
}

func (entity *Terminator) GetIdentitySecret() []byte {
	return entity.IdentitySecret
}

func (entity *Terminator) GetCost() uint16 {
	return entity.Cost
}

func (entity *Terminator) GetPrecedence() xt.Precedence {
	return entity.Precedence
}

func (entity *Terminator) GetPeerData() xt.PeerData {
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
	entity.Identity = boltTerminator.Identity
	entity.IdentitySecret = boltTerminator.IdentitySecret
	entity.PeerData = boltTerminator.PeerData
	entity.Cost = boltTerminator.Cost
	entity.Precedence = xt.GetPrecedenceForName(boltTerminator.Precedence)
	entity.FillCommon(boltTerminator)
	return nil
}

func (entity *Terminator) toBolt() *db.Terminator {
	precedence := xt.Precedences.Default.String()
	if entity.Precedence != nil {
		precedence = entity.Precedence.String()
	}
	return &db.Terminator{
		BaseExtEntity:  *boltz.NewExtEntity(entity.Id, entity.Tags),
		Service:        entity.Service,
		Router:         entity.Router,
		Binding:        entity.Binding,
		Address:        entity.Address,
		Identity:       entity.Identity,
		IdentitySecret: entity.IdentitySecret,
		Cost:           entity.Cost,
		Precedence:     precedence,
		PeerData:       entity.PeerData,
	}
}

func newTerminatorController(controllers *Controllers) *TerminatorController {
	result := &TerminatorController{
		baseController: newController(controllers, controllers.stores.Terminator),
		store:          controllers.stores.Terminator,
	}
	result.impl = result

	controllers.stores.Terminator.On(boltz.EventDelete, func(params ...interface{}) {
		for _, entity := range params {
			if terminator, ok := entity.(*db.Terminator); ok {
				xt.GlobalCosts().ClearCost(terminator.Id)
			}
		}
	})

	xt.GlobalCosts().SetPrecedenceChangeHandler(result.handlePrecedenceChange)

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
		id, err = ctrl.CreateInTx(boltz.NewMutateContext(tx), s)
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

func (ctrl *TerminatorController) CreateInTx(ctx boltz.MutateContext, terminator *Terminator) (string, error) {
	ctrl.checkBinding(terminator)
	boltTerminator := terminator.toBolt()
	if err := ctrl.GetStore().Create(ctx, boltTerminator); err != nil {
		return "", err
	}
	return boltTerminator.Id, nil
}

func (ctrl *TerminatorController) handlePrecedenceChange(terminatorId string, precedence xt.Precedence) {
	terminator, err := ctrl.Read(terminatorId)
	if err != nil {
		pfxlog.Logger().Errorf("unable to update precedence for terminator %v to %v (%v)",
			terminatorId, precedence, err)
		return
	}

	terminator.Precedence = precedence
	checker := boltz.MapFieldChecker{
		db.FieldTerminatorPrecedence: struct{}{},
	}

	if err = ctrl.Patch(terminator, checker); err != nil {
		pfxlog.Logger().Errorf("unable to update precedence for terminator %v to %v (%v)", terminatorId, precedence, err)
	}
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
	*Terminator
}

func (r *RoutingTerminator) GetRouteCost() uint32 {
	return r.RouteCost
}
