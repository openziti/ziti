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

package model

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

func NewTransitRouterHandler(env Env) *TransitRouterHandler {
	handler := &TransitRouterHandler{
		baseHandler: newBaseHandler(env, env.GetStores().TransitRouter),
		allowedFields: boltz.MapFieldChecker{
			persistence.FieldName: struct{}{},
			boltz.FieldTags:       struct{}{},
		},
	}
	handler.impl = handler
	return handler
}

type TransitRouterHandler struct {
	baseHandler
	allowedFields boltz.FieldChecker
}

func (handler *TransitRouterHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *TransitRouterHandler) newModelEntity() boltEntitySink {
	return &TransitRouter{}
}

func (handler *TransitRouterHandler) Create(entity *TransitRouter) (string, error) {
	enrollment := &Enrollment{
		BaseEntity: models.BaseEntity{},
		Method:     MethodEnrollTransitRouterOtt,
	}

	id, _, err := handler.CreateWithEnrollment(entity, enrollment)
	return id, err
}

func (handler *TransitRouterHandler) CreateWithEnrollment(txRouter *TransitRouter, enrollment *Enrollment) (string, string, error) {

	if txRouter.Id == "" {
		txRouter.Id = uuid.New().String()
	}
	var enrollmentId string

	err := handler.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltEntity, err := txRouter.toBoltEntityForCreate(tx, handler.impl)
		if err != nil {
			return err
		}
		if err := handler.GetStore().Create(ctx, boltEntity); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", handler.GetStore().GetSingularEntityType())
			return err
		}

		enrollment.TransitRouterId = &txRouter.Id

		err = enrollment.FillJwtInfo(handler.env, txRouter.Id)

		if err != nil {
			return err
		}

		enrollmentId, err = handler.env.GetHandlers().Enrollment.createEntityInTx(ctx, enrollment)

		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return "", "", err
	}

	return txRouter.Id, enrollmentId, nil
}

func (handler *TransitRouterHandler) Update(entity *TransitRouter, allowAllFields bool) error {
	curEntity, err := handler.Read(entity.Id)

	if err != nil {
		return err
	}

	if curEntity.IsBase {
		return apierror.NewFabricRouterCannotBeUpdate()
	}

	if allowAllFields {
		return handler.updateEntity(entity, nil)
	}

	return handler.updateEntity(entity, handler.allowedFields)

}

func (handler *TransitRouterHandler) Patch(entity *TransitRouter, checker boltz.FieldChecker, allowAllFields bool) error {
	curEntity, err := handler.Read(entity.Id)

	if err != nil {
		return err
	}

	if curEntity.IsBase {
		return apierror.NewFabricRouterCannotBeUpdate()
	}

	if allowAllFields {
		return handler.patchEntity(entity, checker)
	}
	combinedChecker := &AndFieldChecker{first: handler.allowedFields, second: checker}
	return handler.patchEntity(entity, combinedChecker)
}

func (handler *TransitRouterHandler) ReadOneByQuery(query string) (*TransitRouter, error) {
	result, err := handler.readEntityByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*TransitRouter), nil
}

func (handler *TransitRouterHandler) Read(id string) (*TransitRouter, error) {
	result := &TransitRouter{}

	if err := handler.readEntity(id, result); err != nil {
		return nil, err
	}

	return result, nil
}

func (handler *TransitRouterHandler) readInTx(tx *bbolt.Tx, id string) (*TransitRouter, error) {
	modelEntity := &TransitRouter{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *TransitRouterHandler) CollectEnrollments(id string, collector func(entity *Enrollment) error) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.collectEnrollmentsInTx(tx, id, collector)
	})
}

func (handler *TransitRouterHandler) collectEnrollmentsInTx(tx *bbolt.Tx, id string, collector func(entity *Enrollment) error) error {
	_, err := handler.readInTx(tx, id)
	if err != nil {
		return err
	}

	associationIds := handler.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldTransitRouterEnrollments)
	for _, enrollmentId := range associationIds {
		enrollment, err := handler.env.GetHandlers().Enrollment.readInTx(tx, enrollmentId)
		if err != nil {
			return err
		}
		err = collector(enrollment)

		if err != nil {
			return err
		}
	}
	return nil
}
