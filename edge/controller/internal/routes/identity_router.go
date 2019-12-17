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
	"fmt"
	"github.com/netfoundry/ziti-edge/edge/controller/env"
	"github.com/netfoundry/ziti-edge/edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/edge/controller/response"
	"github.com/netfoundry/ziti-edge/edge/migration"

	"net/http"
)

func init() {
	r := NewIdentityRouter()
	env.AddRouter(r)
}

type IdentityRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewIdentityRouter() *IdentityRouter {
	return &IdentityRouter{
		BasePath: "/" + EntityNameIdentity,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *IdentityRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())

	currentIdentityRouter := ae.RootRouter.PathPrefix("/current-identity").Subrouter()
	currentIdentityRouter.HandleFunc("", ae.WrapHandler(detailCurrentUser, permissions.IsAuthenticated())).Methods(http.MethodGet)
	currentIdentityRouter.HandleFunc("/", ae.WrapHandler(detailCurrentUser, permissions.IsAuthenticated())).Methods(http.MethodGet)

	edgeRouterPolicyUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameEdgeRouterPolicy)
	edgeRouterPoliciesListHandler := ae.WrapHandler(ir.ListEdgeRouterPolicies, permissions.IsAdmin())

	sr.HandleFunc(edgeRouterPolicyUrl, edgeRouterPoliciesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(edgeRouterPolicyUrl+"/", edgeRouterPoliciesListHandler).Methods(http.MethodGet)

	servicePolicyUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameServicePolicy)
	servicePoliciesListHandler := ae.WrapHandler(ir.ListServicePolicies, permissions.IsAdmin())

	sr.HandleFunc(servicePolicyUrl, servicePoliciesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(servicePolicyUrl+"/", servicePoliciesListHandler).Methods(http.MethodGet)
}

func detailCurrentUser(ae *env.AppEnv, rc *response.RequestContext) {
	result, err := MapIdentityToApiEntity(ae, rc, rc.Identity)

	if err != nil {
		rc.RequestResponder.RespondWithError(err)
		return
	}
	rc.RequestResponder.RespondWithOk(result, nil)
}

func (ir *IdentityRouter) ToApiListEntity(*env.AppEnv, *response.RequestContext, migration.BaseDbModel) (BaseApiEntity, error) {
	panic("implement me")
}

func (ir *IdentityRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Identity, MapIdentityToApiEntity)
}

func (ir *IdentityRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Identity, MapIdentityToApiEntity, ir.IdType)
}

func (ir *IdentityRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := NewIdentityApiCreate()
	Create(rc, rc.RequestResponder, ae.Schemes.Identity.Post, apiEntity, (&IdentityApiList{}).BuildSelfLink, func() (string, error) {
		identity, enrollments := apiEntity.ToModel()
		identityId, _, err := ae.Handlers.Identity.HandleCreateWithEnrollments(identity, enrollments)
		return identityId, err
	})
}

func (ir *IdentityRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.Identity)
}

func (ir *IdentityRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &IdentityApiUpdate{}
	Update(rc, ae.Schemes.Identity.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.Identity.HandleUpdate(apiEntity.ToModel(id))
	})
}

func (ir *IdentityRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &IdentityApiUpdate{}
	Patch(rc, ae.Schemes.Identity.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.Identity.HandlePatch(apiEntity.ToModel(id), fields.ConcatNestedNames())
	})
}

func (ir *IdentityRouter) ListEdgeRouterPolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.Identity.HandleCollectEdgeRouterPolicies, MapEdgeRouterPolicyToApiEntity)
}

func (ir *IdentityRouter) ListServicePolicies(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.Identity.HandleCollectServicePolicies, MapServicePolicyToApiEntity)
}
