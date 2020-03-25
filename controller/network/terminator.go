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
	"github.com/netfoundry/ziti-fabric/controller/controllers"
	"github.com/netfoundry/ziti-fabric/controller/db"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/netfoundry/ziti-foundation/util/sequence"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"reflect"
	"strings"
	"time"
)

type Terminator struct {
	models.BaseEntity
	Service  string
	Router   string
	Binding  string
	Address  string
	PeerData map[uint32][]byte
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
		PeerData:      entity.PeerData,
	}
}

func newTerminatorController(controllers *Controllers) *TerminatorController {
	result := &TerminatorController{
		baseController: newController(controllers, controllers.stores.Terminator),
		store:          controllers.stores.Terminator,
		sequence:       sequence.NewSequence(),
	}
	result.impl = result
	return result
}

type TerminatorController struct {
	baseController
	store    db.TerminatorStore
	sequence *sequence.Sequence
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

func (ctrl *TerminatorController) createInTx(ctx boltz.MutateContext, e *Terminator) (string, error) {
	if e.Id == "" {
		var err error
		e.Id, err = ctrl.sequence.NextHash()
		if err != nil {
			return "", err
		}
	}
	if e.Binding == "" {
		if strings.HasPrefix(e.Address, "udp:") {
			e.Binding = "udp"
		} else {
			e.Binding = "transport"
		}
	}
	if e.Address == "" {
		return "", models.NewFieldError("required value is missing", "address", e.Binding)
	}
	if !ctrl.stores.Service.IsEntityPresent(ctx.Tx(), e.Service) {
		return "", boltz.NewNotFoundError("service", "service", e.Service)
	}
	if e.Router == "" {
		return "", errors.Errorf("router is required when creating terminator. id: %v, service: %v", e.Id, e.Service)
	}
	if !ctrl.stores.Router.IsEntityPresent(ctx.Tx(), e.Router) {
		return "", boltz.NewNotFoundError("router", "router", e.Router)
	}
	e.CreatedAt = time.Now()
	if err := ctrl.GetStore().Create(ctx, e.toBolt()); err != nil {
		return "", err
	}
	return e.Id, nil
}

func (ctrl *TerminatorController) Update(s *Terminator) error {
	return ctrl.db.Update(func(tx *bbolt.Tx) error {
		return ctrl.GetStore().Update(boltz.NewMutateContext(tx), s.toBolt(), nil)
	})
}

func (ctrl *TerminatorController) Patch(s *Terminator, checker boltz.FieldChecker) error {
	return ctrl.db.Update(func(tx *bbolt.Tx) error {
		return ctrl.GetStore().Update(boltz.NewMutateContext(tx), s.toBolt(), checker)
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
	return controllers.DeleteEntityById(ctrl.GetStore(), ctrl.db, id)
}
