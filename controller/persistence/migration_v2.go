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

package persistence

import "github.com/google/uuid"

func createEdgeRouterPoliciesV2(mtx *MigrationContext) error {
	allPolicy := &EdgeRouterPolicy{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		Name:               "migration policy allowing access to all edge routers and all identities",
		IdentityRoles:      []string{AllRole},
		EdgeRouterRoles:    []string{AllRole},
	}
	err := mtx.Stores.EdgeRouterPolicy.Create(mtx.Ctx, allPolicy)
	if err != nil {
		return err
	}

	edgeRouterIds, _, err := mtx.Stores.EdgeRouter.QueryIds(mtx.Ctx.Tx(), "true")
	if err != nil {
		return err
	}
	for _, edgeRouterId := range edgeRouterIds {
		edgeRouter, err := mtx.Stores.EdgeRouter.LoadOneById(mtx.Ctx.Tx(), edgeRouterId)
		if err != nil {
			return err
		}
		if edgeRouter.ClusterId == nil {
			continue
		}
		cluster, err := mtx.Stores.Cluster.LoadOneById(mtx.Ctx.Tx(), *edgeRouter.ClusterId)
		if err != nil {
			return err
		}
		edgeRouter.RoleAttributes = append(edgeRouter.RoleAttributes, "cluster-"+cluster.Name)
		if err = mtx.Stores.EdgeRouter.Update(mtx.Ctx, edgeRouter, nil); err != nil {
			return err
		}
	}

	clusterIds, _, err := mtx.Stores.Cluster.QueryIds(mtx.Ctx.Tx(), "true")
	if err != nil {
		return err
	}
	for _, clusterId := range clusterIds {
		name := string(mtx.Stores.Cluster.GetNameIndex().Read(mtx.Ctx.Tx(), []byte(clusterId)))
		serviceIds := mtx.Stores.Cluster.GetRelatedEntitiesIdList(mtx.Ctx.Tx(), clusterId, EntityTypeServices)
		edgeRouterIds := mtx.Stores.Cluster.GetRelatedEntitiesIdList(mtx.Ctx.Tx(), clusterId, EntityTypeEdgeRouters)

		serviceEdgeRouterPolicy := newServiceEdgeRouterPolicy(name)
		for _, serviceId := range serviceIds {
			serviceEdgeRouterPolicy.ServiceRoles = append(serviceEdgeRouterPolicy.ServiceRoles, "@"+serviceId)
		}

		for _, edgeRouterId := range edgeRouterIds {
			serviceEdgeRouterPolicy.EdgeRouterRoles = append(serviceEdgeRouterPolicy.ServiceRoles, "@"+edgeRouterId)
		}
		if err := mtx.Stores.ServiceEdgeRouterPolicy.Create(mtx.Ctx, serviceEdgeRouterPolicy); err != nil {
			return err
		}
	}

	return nil
}
