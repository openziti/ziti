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

package routes

import (
	"github.com/netfoundry/ziti-edge/edge/controller/env"
	"github.com/netfoundry/ziti-edge/edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/edge/controller/response"
	"fmt"
	"net/http"
)

func init() {
	r := NewClusterRouter()
	env.AddRouter(r)
}

type ClusterRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewClusterRouter() *ClusterRouter {
	return &ClusterRouter{
		BasePath: "/" + EntityNameCluster,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *ClusterRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())

	edgeRouterUrlWithSlash := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameEdgeRouter)
	gatewayUrlWithSlash :=    fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameGateway)

	edgeRouterWithOutSlash := fmt.Sprintf("/{%s}/%s/", response.IdPropertyName, EntityNameEdgeRouter)
	gatewayUrlWithOutSlash :=  fmt.Sprintf("/{%s}/%s/", response.IdPropertyName, EntityNameGateway)

	edgeRoutersListHandler := ae.WrapHandler(ir.ListEdgeRouters, permissions.IsAdmin())
	edgeRoutersAddHandler := ae.WrapHandler(ir.AddEdgeRouters, permissions.IsAdmin())

	//gets
	sr.HandleFunc(gatewayUrlWithSlash, edgeRoutersListHandler).Methods(http.MethodGet)
	sr.HandleFunc(edgeRouterUrlWithSlash, edgeRoutersListHandler).Methods(http.MethodGet)

	sr.HandleFunc(gatewayUrlWithOutSlash, edgeRoutersListHandler).Methods(http.MethodGet)
	sr.HandleFunc(edgeRouterWithOutSlash, edgeRoutersListHandler).Methods(http.MethodGet)

	//puts
	sr.HandleFunc(gatewayUrlWithSlash, edgeRoutersAddHandler).Methods(http.MethodPut)
	sr.HandleFunc(edgeRouterUrlWithSlash, edgeRoutersAddHandler).Methods(http.MethodPut)

	sr.HandleFunc(gatewayUrlWithOutSlash, edgeRoutersAddHandler).Methods(http.MethodPut)
	sr.HandleFunc(edgeRouterWithOutSlash, edgeRoutersAddHandler).Methods(http.MethodPut)

}

func (ir *ClusterRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Cluster, MapClusterToApiEntity)
}

func (ir *ClusterRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Cluster, MapClusterToApiEntity, ir.IdType)
}

func (ir *ClusterRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ClusterApi{}
	Create(rc, rc.RequestResponder, ae.Schemes.Cluster.Post, apiEntity, (&ClusterApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.Cluster.HandleCreate(apiEntity.ToModel(""))
	})
}

func (ir *ClusterRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.Cluster)
}

func (ir *ClusterRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ClusterApi{}
	Update(rc, ae.Schemes.Cluster.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.Cluster.HandleUpdate(apiEntity.ToModel(id))
	})
}

func (ir *ClusterRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &ClusterApi{}
	Patch(rc, ae.Schemes.Cluster.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.Cluster.HandlePatch(apiEntity.ToModel(id), fields.ConcatNestedNames())
	})
}

func (ir *ClusterRouter) ListEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.Cluster.HandleCollectEdgeRouters, MapEdgeRouterToApiEntity)
}

func (ir *ClusterRouter) AddEdgeRouters(ae *env.AppEnv, rc *response.RequestContext) {
	UpdateAssociations(ae, rc, ir.IdType, func(parentId string, childIds []string) error {
		return ae.Handlers.Cluster.HandleAddEdgeRouters(parentId, childIds)
	})
}