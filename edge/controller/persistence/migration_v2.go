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
		IdentityRoles:      []string{"@all"},
		EdgeRouterRoles:    []string{"@all"},
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

	serviceIds, _, err := mtx.Stores.EdgeService.QueryIds(mtx.Ctx.Tx(), "true")
	if err != nil {
		return err
	}
	for _, serviceId := range serviceIds {
		service, err := mtx.Stores.EdgeService.LoadOneById(mtx.Ctx.Tx(), serviceId)
		if err != nil {
			return err
		}
		clusterIds := mtx.Stores.EdgeService.GetRelatedEntitiesIdList(mtx.Ctx.Tx(), serviceId, FieldServiceClusters)
		if len(clusterIds) > 0 {
			cluster, err := mtx.Stores.Cluster.LoadOneById(mtx.Ctx.Tx(), clusterIds[0])
			if err != nil {
				return err
			}
			service.EdgeRouterRoles = append(service.EdgeRouterRoles, "@cluster-"+cluster.Name)
			if err = mtx.Stores.EdgeService.Update(mtx.Ctx, service, nil); err != nil {
				return err
			}
		} else if len(clusterIds) > 1 {
			for _, clusterId := range clusterIds[1:] {
				edgeRouterIds := mtx.Stores.Cluster.GetRelatedEntitiesIdList(mtx.Ctx.Tx(), clusterId, FieldClusterEdgeRouters)
				service.EdgeRouterRoles = append(service.EdgeRouterRoles, edgeRouterIds...)
				if err = mtx.Stores.EdgeService.Update(mtx.Ctx, service, nil); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
