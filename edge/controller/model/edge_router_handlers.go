/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-edge/edge/controller/apierror"
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/edge/controller/util"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"fmt"
	"go.etcd.io/bbolt"
)

func NewEdgeRouterHandler(env Env) *EdgeRouterHandler {
	handler := &EdgeRouterHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().EdgeRouter,
		},
		allowedFieldsChecker: boltz.MapFieldChecker{
			persistence.FieldName:              struct{}{},
			persistence.FieldEdgeRouterCluster: struct{}{},
			persistence.FieldTags:              struct{}{},
		},
	}
	handler.impl = handler
	return handler
}

type EdgeRouterHandler struct {
	baseHandler
	allowedFieldsChecker boltz.FieldChecker
}

func (handler *EdgeRouterHandler) NewModelEntity() BaseModelEntity {
	return &EdgeRouter{}
}

func (handler *EdgeRouterHandler) HandleCreate(modelEntity *EdgeRouter) (string, error) {
	cluster, err := handler.env.GetHandlers().Cluster.HandleRead(modelEntity.ClusterId)

	if err != nil && !util.IsErrNotFoundErr(err) {
		return "", err
	}

	if cluster == nil {
		apiErr := apierror.NewNotFound()
		apiErr.Cause = NewFieldError("clusterId not found", "clusterId", modelEntity.ClusterId)
		apiErr.AppendCause = true
		return "", apiErr
	}

	return handler.create(modelEntity, nil)
}

func (handler *EdgeRouterHandler) HandleRead(id string) (*EdgeRouter, error) {
	modelEntity := &EdgeRouter{}
	if err := handler.read(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *EdgeRouterHandler) handleReadInTx(tx *bbolt.Tx, id string) (*EdgeRouter, error) {
	modelEntity := &EdgeRouter{}
	if err := handler.readInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *EdgeRouterHandler) HandleReadOneByQuery(query string) (*EdgeRouter, error) {
	result, err := handler.readByQuery(query)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return result.(*EdgeRouter), nil
}

func (handler *EdgeRouterHandler) HandleReadOneByFingerprint(fingerprint string) (*EdgeRouter, error) {
	return handler.HandleReadOneByQuery(fmt.Sprintf(`fingerprint = "%v"`, fingerprint))
}

func (handler *EdgeRouterHandler) HandleUpdate(modelEntity *EdgeRouter, restrictFields bool) error {
	if restrictFields {
		return handler.update(modelEntity, handler.allowedFieldsChecker, nil)
	}
	return handler.update(modelEntity, nil, nil)
}

func (handler *EdgeRouterHandler) HandlePatch(modelEntity *EdgeRouter, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler.allowedFieldsChecker, second: checker}
	return handler.patch(modelEntity, combinedChecker, nil)
}

func (handler *EdgeRouterHandler) beforeDelete(tx *bbolt.Tx, id string) error {
	store := handler.GetDbProvider().GetRouterStore()
	if store.IsEntityPresent(tx, id) {
		return store.DeleteById(boltz.NewMutateContext(tx), id)
	}
	return nil
}

func (handler *EdgeRouterHandler) HandleDelete(id string) error {
	return handler.delete(id, handler.beforeDelete, nil)
}

func (handler *EdgeRouterHandler) HandleQuery(query string) (*EdgeRouterListResult, error) {
	result := &EdgeRouterListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *EdgeRouterHandler) HandleList(queryOptions *QueryOptions) (*EdgeRouterListResult, error) {
	result := &EdgeRouterListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type EdgeRouterListResult struct {
	handler     *EdgeRouterHandler
	EdgeRouters []*EdgeRouter
	QueryMetaData
}

func (result *EdgeRouterListResult) collect(tx *bbolt.Tx, ids [][]byte, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.handleReadInTx(tx, string(key))
		if err != nil {
			return err
		}
		result.EdgeRouters = append(result.EdgeRouters, entity)
	}
	return nil
}
