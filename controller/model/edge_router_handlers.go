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
	"fmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"strconv"
)

func NewEdgeRouterHandler(env Env) *EdgeRouterHandler {
	handler := &EdgeRouterHandler{
		baseHandler: newBaseHandler(env, env.GetStores().EdgeRouter),
		allowedFieldsChecker: boltz.MapFieldChecker{
			persistence.FieldName:           struct{}{},
			persistence.FieldRoleAttributes: struct{}{},
			boltz.FieldTags:                 struct{}{},
		},
	}
	handler.impl = handler
	return handler
}

type EdgeRouterHandler struct {
	baseHandler
	allowedFieldsChecker boltz.FieldChecker
}

func (handler *EdgeRouterHandler) newModelEntity() boltEntitySink {
	return &EdgeRouter{}
}

func (handler *EdgeRouterHandler) Create(modelEntity *EdgeRouter) (string, error) {
	enrollment := &Enrollment{
		BaseEntity: models.BaseEntity{},
		Method:     MethodEnrollEdgeRouterOtt,
	}

	id, _, err := handler.CreateWithEnrollment(modelEntity, enrollment)

	return id, err
}

func (handler *EdgeRouterHandler) Read(id string) (*EdgeRouter, error) {
	modelEntity := &EdgeRouter{}
	if err := handler.readEntity(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *EdgeRouterHandler) readInTx(tx *bbolt.Tx, id string) (*EdgeRouter, error) {
	modelEntity := &EdgeRouter{}
	if err := handler.readEntityInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *EdgeRouterHandler) ReadOneByQuery(query string) (*EdgeRouter, error) {
	result, err := handler.readEntityByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*EdgeRouter), nil
}

func (handler *EdgeRouterHandler) ReadOneByFingerprint(fingerprint string) (*EdgeRouter, error) {
	return handler.ReadOneByQuery(fmt.Sprintf(`fingerprint = "%v"`, fingerprint))
}

func (handler *EdgeRouterHandler) Update(modelEntity *EdgeRouter, restrictFields bool) error {
	if restrictFields {
		return handler.updateEntity(modelEntity, handler.allowedFieldsChecker)
	}
	return handler.updateEntity(modelEntity, nil)
}

func (handler *EdgeRouterHandler) Patch(modelEntity *EdgeRouter, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler.allowedFieldsChecker, second: checker}
	return handler.patchEntity(modelEntity, combinedChecker)
}

func (handler *EdgeRouterHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *EdgeRouterHandler) Query(query string) (*EdgeRouterListResult, error) {
	result := &EdgeRouterListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *EdgeRouterHandler) ListForSession(sessionId string) (*EdgeRouterListResult, error) {
	var result *EdgeRouterListResult

	err := handler.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		session, err := handler.env.GetStores().Session.LoadOneById(tx, sessionId)
		if err != nil {
			return err
		}
		apiSession, err := handler.env.GetStores().ApiSession.LoadOneById(tx, session.ApiSessionId)
		if err != nil {
			return err
		}

		result, err = handler.ListForIdentityAndServiceWithTx(tx, apiSession.IdentityId, session.ServiceId, nil)
		return err
	})
	return result, err
}

func (handler *EdgeRouterHandler) ListForIdentityAndServiceWithTx(tx *bbolt.Tx, identityId, serviceId string, limit *int) (*EdgeRouterListResult, error) {
	service, err := handler.env.GetStores().EdgeService.LoadOneById(tx, serviceId)
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, errors.Errorf("no service with id %v found", serviceId)
	}

	query := fmt.Sprintf(`anyOf(edgeRouterPolicies.identities) = "%v" and anyOf(serviceEdgeRouterPolicies.services) = "%v"`, identityId, service.Id)

	if limit != nil {
		query += " limit " + strconv.Itoa(*limit)
	}

	result := &EdgeRouterListResult{handler: handler}
	if err = handler.ListWithTx(tx, query, result.collect); err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *EdgeRouterHandler) QueryRoleAttributes(queryString string) ([]string, *models.QueryMetaData, error) {
	index := handler.env.GetStores().EdgeRouter.GetRoleAttributesIndex()
	return handler.queryRoleAttributes(index, queryString)
}

func (handler *EdgeRouterHandler) CreateWithEnrollment(edgeRouter *EdgeRouter, enrollment *Enrollment) (string, string, error) {
	if edgeRouter.Id == "" {
		edgeRouter.Id = uuid.New().String()
	}

	if enrollment.Id == "" {
		enrollment.Id = uuid.New().String()
	}

	err := handler.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		boltEdgeRouter, err := edgeRouter.toBoltEntityForCreate(tx, handler.impl)
		if err != nil {
			return err
		}
		if err := handler.GetStore().Create(ctx, boltEdgeRouter); err != nil {
			pfxlog.Logger().WithError(err).Errorf("could not create %v in bolt storage", handler.GetStore().GetSingularEntityType())
			return err
		}

		enrollment.EdgeRouterId = &edgeRouter.Id

		err = enrollment.FillJwtInfo(handler.env, edgeRouter.Id)

		if err != nil {
			return err
		}

		_, err = handler.env.GetHandlers().Enrollment.createEntityInTx(ctx, enrollment)

		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return "", "", err
	}

	return edgeRouter.Id, enrollment.Id, nil
}

func (handler *EdgeRouterHandler) CollectEnrollments(id string, collector func(entity *Enrollment) error) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		return handler.collectEnrollmentsInTx(tx, id, collector)
	})
}

func (handler *EdgeRouterHandler) collectEnrollmentsInTx(tx *bbolt.Tx, id string, collector func(entity *Enrollment) error) error {
	_, err := handler.readInTx(tx, id)
	if err != nil {
		return err
	}

	associationIds := handler.GetStore().GetRelatedEntitiesIdList(tx, id, persistence.FieldEdgeRouterEnrollments)
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

type EdgeRouterListResult struct {
	handler     *EdgeRouterHandler
	EdgeRouters []*EdgeRouter
	models.QueryMetaData
}

func (result *EdgeRouterListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.EdgeRouters = append(result.EdgeRouters, entity)
	}
	return nil
}
