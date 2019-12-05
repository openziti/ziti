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
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"go.etcd.io/bbolt"
)

func NewClusterHandler(env Env) *ClusterHandler {
	handler := &ClusterHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().Cluster,
		},
	}
	handler.impl = handler
	return handler
}

type ClusterHandler struct {
	baseHandler
}

func (handler *ClusterHandler) NewModelEntity() BaseModelEntity {
	return &Cluster{}
}

func (handler *ClusterHandler) HandleCreate(clusterModel *Cluster) (string, error) {
	return handler.create(clusterModel, nil)
}

func (handler *ClusterHandler) HandleRead(id string) (*Cluster, error) {
	modelEntity := &Cluster{}
	if err := handler.read(id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ClusterHandler) handleReadInTx(tx *bbolt.Tx, id string) (*Cluster, error) {
	modelEntity := &Cluster{}
	if err := handler.readInTx(tx, id, modelEntity); err != nil {
		return nil, err
	}
	return modelEntity, nil
}

func (handler *ClusterHandler) IsUpdated(field string) bool {
	return field != "Services" && field != "Identities"
}

func (handler *ClusterHandler) HandleUpdate(cluster *Cluster) error {
	return handler.update(cluster, handler, nil)
}

func (handler *ClusterHandler) HandlePatch(cluster *Cluster, checker boltz.FieldChecker) error {
	combinedChecker := &AndFieldChecker{first: handler, second: checker}
	return handler.patch(cluster, combinedChecker, nil)
}

func (handler *ClusterHandler) HandleDelete(id string) error {
	return handler.delete(id, nil, nil)
}

func (handler *ClusterHandler) HandleList(queryOptions *QueryOptions) (*ClusterListResult, error) {
	result := &ClusterListResult{handler: handler}
	err := handler.parseAndList(queryOptions, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ClusterHandler) HandleListEdgeRouters(id string) ([]*EdgeRouter, error) {
	var result []*EdgeRouter
	err := handler.HandleCollectEdgeRouters(id, func(entity BaseModelEntity) {
		result = append(result, entity.(*EdgeRouter))
	})

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *ClusterHandler) HandleCollectEdgeRouters(id string, collector func(entity BaseModelEntity)) error {
	return handler.GetDb().View(func(tx *bbolt.Tx) error {
		_, err := handler.handleReadInTx(tx, id)
		if err != nil {
			return err
		}
		edgeRouterIds := handler.store.GetRelatedEntitiesIdList(tx, id, persistence.FieldClusterEdgeRouters)
		for _, edgeRouterId := range edgeRouterIds {
			edgeRouter, err := handler.env.GetHandlers().EdgeRouter.handleReadInTx(tx, edgeRouterId)
			if err != nil {
				return err
			}
			collector(edgeRouter)
		}
		return nil
	})
}

func (handler *ClusterHandler) HandleAddEdgeRouters(id string, edgeRouterIds []string) error {
	return handler.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		if _, err := handler.BaseLoad(id); err != nil {
			return err
		}
		for _, gwId := range edgeRouterIds {
			if err := handler.env.GetStores().EdgeRouter.UpdateCluster(ctx, gwId, id); err != nil {
				return err
			}
		}
		return nil
	})
}

type ClusterListResult struct {
	handler  *ClusterHandler
	Clusters []*Cluster
	QueryMetaData
}

func (result *ClusterListResult) collect(tx *bbolt.Tx, ids [][]byte, queryMetaData *QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.handleReadInTx(tx, string(key))
		if err != nil {
			return err
		}
		result.Clusters = append(result.Clusters, entity)
	}
	return nil
}
