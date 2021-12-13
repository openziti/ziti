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
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

type Controllers struct {
	db          boltz.Db
	stores      *db.Stores
	Terminators *TerminatorController
	Routers     *RouterController
	Services    *ServiceController
	Inspections *InspectionsController
}

func (e *Controllers) getDb() boltz.Db {
	return e.db
}

func NewControllers(db boltz.Db, stores *db.Stores) *Controllers {
	result := &Controllers{
		db:     db,
		stores: stores,
	}
	result.Terminators = newTerminatorController(result)
	result.Routers = newRouterController(result)
	result.Services = newServiceController(result)
	result.Inspections = &InspectionsController{}
	return result
}

type Controller interface {
	models.EntityRetriever
	getControllers() *Controllers

	newModelEntity() boltEntitySink
	readEntityInTx(tx *bbolt.Tx, id string, modelEntity boltEntitySink) error
}

type boltEntitySink interface {
	models.Entity
	fillFrom(controller Controller, tx *bbolt.Tx, boltEntity boltz.Entity) error
}

func newController(controllers *Controllers, store boltz.CrudStore) baseController {
	return baseController{
		BaseController: models.BaseController{
			Store: store,
		},
		Controllers: controllers,
	}
}

type baseController struct {
	models.BaseController
	*Controllers
	impl Controller
}

func (ctrl *baseController) BaseLoad(id string) (models.Entity, error) {
	entity := ctrl.impl.newModelEntity()
	if err := ctrl.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (ctrl *baseController) BaseLoadInTx(tx *bbolt.Tx, id string) (models.Entity, error) {
	entity := ctrl.impl.newModelEntity()
	if err := ctrl.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (ctrl *baseController) getControllers() *Controllers {
	return ctrl.Controllers
}

func (ctrl *baseController) readEntity(id string, modelEntity boltEntitySink) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.readEntityInTx(tx, id, modelEntity)
	})
}

func (ctrl *baseController) readEntityInTx(tx *bbolt.Tx, id string, modelEntity boltEntitySink) error {
	boltEntity := ctrl.impl.GetStore().NewStoreEntity()
	found, err := ctrl.impl.GetStore().BaseLoadOneById(tx, id, boltEntity)
	if err != nil {
		return err
	}
	if !found {
		return boltz.NewNotFoundError(ctrl.impl.GetStore().GetSingularEntityType(), "id", id)
	}

	return modelEntity.fillFrom(ctrl.impl, tx, boltEntity)
}

func (ctrl *baseController) BaseList(query string) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: ctrl}
	err := ctrl.list(query, result.Collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (ctrl *baseController) list(queryString string, resultHandler models.ListResultHandler) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.ListWithTx(tx, queryString, resultHandler)
	})
}

func (ctrl *baseController) BasePreparedList(query ast.Query) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: ctrl}
	err := ctrl.preparedList(query, result.Collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (ctrl *baseController) preparedList(query ast.Query, resultHandler models.ListResultHandler) error {
	return ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.PreparedListWithTx(tx, query, resultHandler)
	})
}

func (ctrl *baseController) BasePreparedListAssociated(id string, typeLoader models.EntityRetriever, query ast.Query) (*models.EntityListResult, error) {
	result := &models.EntityListResult{Loader: ctrl}
	err := ctrl.db.View(func(tx *bbolt.Tx) error {
		return ctrl.PreparedListAssociatedWithTx(tx, id, typeLoader.GetStore().GetEntityType(), query, result.Collect)
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

type boltEntitySource interface {
	models.Entity
	toBolt() boltz.Entity
}

func (ctrl *baseController) updateGeneral(modelEntity boltEntitySource, checker boltz.FieldChecker) error {
	return ctrl.db.Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		existing := ctrl.GetStore().NewStoreEntity()
		found, err := ctrl.GetStore().BaseLoadOneById(tx, modelEntity.GetId(), existing)
		if err != nil {
			return err
		}
		if !found {
			return boltz.NewNotFoundError(ctrl.GetStore().GetSingularEntityType(), "id", modelEntity.GetId())
		}

		boltEntity := modelEntity.toBolt()

		if err := ctrl.ValidateNameOnUpdate(ctx, boltEntity, existing, checker); err != nil {
			return err
		}

		if err := ctrl.GetStore().Update(ctx, boltEntity, checker); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not update %v entity", ctrl.GetStore().GetEntityType())
			return err
		}
		return nil
	})
}
