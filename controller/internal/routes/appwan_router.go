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
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/response"
	"net/http"
)

func init() {
	r := NewAppWanRouter()
	env.AddRouter(r)
}

type AppWanRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewAppWanRouter() *AppWanRouter {
	return &AppWanRouter{
		BasePath: "/" + EntityNameAppWan,
		IdType:   response.IdTypeUuid,
	}
}

func (ir *AppWanRouter) Register(ae *env.AppEnv) {
	sr := registerCrudRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.IsAdmin())

	serviceUrlWithSlash := fmt.Sprintf("/{%s}/services", response.IdPropertyName)
	serviceUrlWithOutSlash := fmt.Sprintf("/{%s}/services/", response.IdPropertyName)

	servicesListHandler := ae.WrapHandler(ir.ListServices, permissions.IsAdmin())
	servicesAddHandler := ae.WrapHandler(ir.AddServices, permissions.IsAdmin())
	servicesRemoveHandler := ae.WrapHandler(ir.RemoveServicesBulk, permissions.IsAdmin())
	servicesSetHandler := ae.WrapHandler(ir.SetServices, permissions.IsAdmin())

	sr.HandleFunc(serviceUrlWithSlash, servicesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(serviceUrlWithOutSlash, servicesListHandler).Methods(http.MethodGet)

	sr.HandleFunc(serviceUrlWithSlash, servicesAddHandler).Methods(http.MethodPut)
	sr.HandleFunc(serviceUrlWithOutSlash, servicesAddHandler).Methods(http.MethodPut)

	sr.HandleFunc(serviceUrlWithSlash, servicesRemoveHandler).Methods(http.MethodDelete)
	sr.HandleFunc(serviceUrlWithOutSlash, servicesRemoveHandler).Methods(http.MethodDelete)

	sr.HandleFunc(serviceUrlWithSlash, servicesSetHandler).Methods(http.MethodPost)
	sr.HandleFunc(serviceUrlWithOutSlash, servicesSetHandler).Methods(http.MethodPost)

	identityUrlWithSlash := fmt.Sprintf("/{%s}/identities", response.IdPropertyName)
	identityUrlWithOutSlash := fmt.Sprintf("/{%s}/identities/", response.IdPropertyName)

	identitiesListHandler := ae.WrapHandler(ir.ListIdentities, permissions.IsAdmin())
	identitiesAddHandler := ae.WrapHandler(ir.AddIdentities, permissions.IsAdmin())
	identitiesBulkRemoveHandler := ae.WrapHandler(ir.RemoveIdentitiesBulk, permissions.IsAdmin())
	identitiesSetHandler := ae.WrapHandler(ir.SetIdentities, permissions.IsAdmin())

	sr.HandleFunc(identityUrlWithSlash, identitiesListHandler).Methods(http.MethodGet)
	sr.HandleFunc(identityUrlWithOutSlash, identitiesListHandler).Methods(http.MethodGet)

	sr.HandleFunc(identityUrlWithSlash, identitiesAddHandler).Methods(http.MethodPut)
	sr.HandleFunc(identityUrlWithOutSlash, identitiesAddHandler).Methods(http.MethodPut)

	sr.HandleFunc(identityUrlWithSlash, identitiesBulkRemoveHandler).Methods(http.MethodDelete)
	sr.HandleFunc(identityUrlWithOutSlash, identitiesBulkRemoveHandler).Methods(http.MethodDelete)

	sr.HandleFunc(identityUrlWithSlash, identitiesSetHandler).Methods(http.MethodPost)
	sr.HandleFunc(identityUrlWithOutSlash, identitiesSetHandler).Methods(http.MethodPost)

	identityUrlWithSlashWithSubId := fmt.Sprintf("/{%s}/identities/{%s}", response.IdPropertyName, response.SubIdPropertyName)
	identityUrlWithOutSlashWithSubId := fmt.Sprintf("/{%s}/identities/{%s}/", response.IdPropertyName, response.SubIdPropertyName)

	identityRemoveHandler := ae.WrapHandler(ir.RemoveIdentity, permissions.IsAdmin())

	sr.HandleFunc(identityUrlWithSlashWithSubId, identityRemoveHandler).Methods(http.MethodDelete)
	sr.HandleFunc(identityUrlWithOutSlashWithSubId, identityRemoveHandler).Methods(http.MethodDelete)

	serviceUrlWithSlashWithSubId := fmt.Sprintf("/{%s}/services/{%s}", response.IdPropertyName, response.SubIdPropertyName)
	serviceUrlWithOutSlashWithSubId := fmt.Sprintf("/{%s}/services/{%s}/", response.IdPropertyName, response.SubIdPropertyName)

	serviceRemoveHandler := ae.WrapHandler(ir.RemoveService, permissions.IsAdmin())

	sr.HandleFunc(serviceUrlWithSlashWithSubId, serviceRemoveHandler).Methods(http.MethodDelete)
	sr.HandleFunc(serviceUrlWithOutSlashWithSubId, serviceRemoveHandler).Methods(http.MethodDelete)
}

func (ir *AppWanRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler(ae, rc, ae.Handlers.Appwan, MapAppWanToApiEntity)
}

func (ir *AppWanRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler(ae, rc, ae.Handlers.Appwan, MapAppWanToApiEntity, ir.IdType)
}

func (ir *AppWanRouter) Create(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := NewAppWanApiCreate()
	Create(rc, rc.RequestResponder, ae.Schemes.AppWan.Post, apiEntity, (&AppWanApiList{}).BuildSelfLink, func() (string, error) {
		return ae.Handlers.Appwan.HandleCreate(apiEntity.ToModel())
	})
}

func (ir *AppWanRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	DeleteWithHandler(rc, ir.IdType, ae.Handlers.Appwan)
}

func (ir *AppWanRouter) Update(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &AppWanApiUpdate{}
	Update(rc, ae.Schemes.AppWan.Put, ir.IdType, apiEntity, func(id string) error {
		return ae.Handlers.Appwan.HandleUpdate(apiEntity.ToModel(id))
	})
}

func (ir *AppWanRouter) Patch(ae *env.AppEnv, rc *response.RequestContext) {
	apiEntity := &AppWanApiUpdate{}
	Patch(rc, ae.Schemes.AppWan.Patch, ir.IdType, apiEntity, func(id string, fields JsonFields) error {
		return ae.Handlers.Appwan.HandlePatch(apiEntity.ToModel(id), fields.ConcatNestedNames())
	})
}

func (ir *AppWanRouter) ListServices(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.Appwan.HandleCollectServices, MapServiceToApiEntity)
}

func (ir *AppWanRouter) AddServices(ae *env.AppEnv, rc *response.RequestContext) {
	UpdateAssociationsFor(ae, rc, ir.IdType, ae.GetStores().Appwan, model.AssociationsActionAdd, persistence.FieldAppwanServices)
}

func (ir *AppWanRouter) RemoveServicesBulk(ae *env.AppEnv, rc *response.RequestContext) {
	UpdateAssociationsFor(ae, rc, ir.IdType, ae.GetStores().Appwan, model.AssociationsActionRemove, persistence.FieldAppwanServices)
}

func (ir *AppWanRouter) RemoveService(ae *env.AppEnv, rc *response.RequestContext) {
	RemoveAssociationFor(ae, rc, ir.IdType, ae.GetStores().Appwan, persistence.FieldAppwanServices)
}

func (ir *AppWanRouter) SetServices(ae *env.AppEnv, rc *response.RequestContext) {
	UpdateAssociationsFor(ae, rc, ir.IdType, ae.GetStores().Appwan, model.AssociationsActionSet, persistence.FieldAppwanServices)
}

func (ir *AppWanRouter) ListIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	ListAssociations(ae, rc, ir.IdType, ae.Handlers.Appwan.HandleCollectIdentities, MapIdentityToApiEntity)
}

func (ir *AppWanRouter) AddIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	UpdateAssociationsFor(ae, rc, ir.IdType, ae.BoltStores.Appwan, model.AssociationsActionAdd, persistence.FieldAppwanIdentities)
}

func (ir *AppWanRouter) RemoveIdentitiesBulk(ae *env.AppEnv, rc *response.RequestContext) {
	UpdateAssociationsFor(ae, rc, ir.IdType, ae.BoltStores.Appwan, model.AssociationsActionRemove, persistence.FieldAppwanIdentities)
}

func (ir *AppWanRouter) RemoveIdentity(ae *env.AppEnv, rc *response.RequestContext) {
	RemoveAssociationFor(ae, rc, ir.IdType, ae.BoltStores.Appwan, persistence.FieldAppwanIdentities)
}

func (ir *AppWanRouter) SetIdentities(ae *env.AppEnv, rc *response.RequestContext) {
	UpdateAssociationsFor(ae, rc, ir.IdType, ae.BoltStores.Appwan, model.AssociationsActionSet, persistence.FieldAppwanIdentities)
}
