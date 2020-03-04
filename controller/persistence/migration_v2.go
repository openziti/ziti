/*
	Copyright 2020 NetFoundry, Inc.

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

import (
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
)

func (m *Migrations) createEdgeRouterPoliciesV2(step *boltz.MigrationStep) {
	_, serviceCount, err := m.stores.EdgeService.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)

	_, routerCount, err := m.stores.EdgeRouter.QueryIds(step.Ctx.Tx(), "true")

	// Only continue if there are services that might stop working if we don't create default or migrated policies
	if step.SetError(err) || serviceCount == 0 || routerCount == 0 {
		return
	}

	allPolicy := &EdgeRouterPolicy{
		BaseExtEntity:   boltz.BaseExtEntity{Id: uuid.New().String()},
		Name:            "migration policy allowing access to all edge routers and all identities",
		IdentityRoles:   []string{AllRole},
		EdgeRouterRoles: []string{AllRole},
	}
	step.SetError(m.stores.EdgeRouterPolicy.Create(step.Ctx, allPolicy))

	edgeRouterIds, _, err := m.stores.EdgeRouter.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)

	for _, edgeRouterId := range edgeRouterIds {
		edgeRouter, err := m.stores.EdgeRouter.LoadOneById(step.Ctx.Tx(), edgeRouterId)
		if step.SetError(err) {
			return
		}
		if edgeRouter.ClusterId == nil {
			continue
		}
		cluster, err := m.stores.Cluster.LoadOneById(step.Ctx.Tx(), *edgeRouter.ClusterId)
		step.SetError(err)
		edgeRouter.RoleAttributes = append(edgeRouter.RoleAttributes, "cluster-"+cluster.Name)
		step.SetError(m.stores.EdgeRouter.Update(step.Ctx, edgeRouter, nil))
	}

	clusterIds, _, err := m.stores.Cluster.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)
	for _, clusterId := range clusterIds {
		name := string(m.stores.Cluster.GetNameIndex().Read(step.Ctx.Tx(), []byte(clusterId)))
		serviceIds := m.stores.Cluster.GetRelatedEntitiesIdList(step.Ctx.Tx(), clusterId, EntityTypeServices)
		edgeRouterIds := m.stores.Cluster.GetRelatedEntitiesIdList(step.Ctx.Tx(), clusterId, EntityTypeEdgeRouters)

		serviceEdgeRouterPolicy := newServiceEdgeRouterPolicy(name)
		for _, serviceId := range serviceIds {
			serviceEdgeRouterPolicy.ServiceRoles = append(serviceEdgeRouterPolicy.ServiceRoles, "@"+serviceId)
		}

		for _, edgeRouterId := range edgeRouterIds {
			serviceEdgeRouterPolicy.EdgeRouterRoles = append(serviceEdgeRouterPolicy.ServiceRoles, "@"+edgeRouterId)
		}
		if step.SetError(m.stores.ServiceEdgeRouterPolicy.Create(step.Ctx, serviceEdgeRouterPolicy)) {
			return
		}
	}
}
