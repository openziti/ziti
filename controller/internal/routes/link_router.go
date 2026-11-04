/*
	Copyright NetFoundry Inc.

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
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/fields"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/openziti/ziti/v2/controller/response"
	"github.com/openziti/ziti/v2/controller/rest_server/operations/link"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

func init() {
	r := NewLinkRouter()
	env.AddRouter(r)
}

type LinkRouter struct {
	BasePath string
}

func NewLinkRouter() *LinkRouter {
	return &LinkRouter{
		BasePath: "/" + EntityNameLink,
	}
}

func (r *LinkRouter) Register(ae *env.AppEnv) {
	ae.FabricApi.LinkDetailLinkHandler = link.DetailLinkHandlerFunc(func(params link.DetailLinkParams, _ any) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, permissions.Ops, permissions.Read)
		return ae.IsAllowed(r.Detail, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.FabricApi.LinkListLinksHandler = link.ListLinksHandlerFunc(func(params link.ListLinksParams, _ any) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, permissions.Ops, permissions.Read)
		return ae.IsAllowed(r.ListLinks, params.HTTPRequest, "", "", permissions.DefaultManagementAccess())
	})

	ae.FabricApi.LinkPatchLinkHandler = link.PatchLinkHandlerFunc(func(params link.PatchLinkParams, _ any) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, permissions.Ops, permissions.Update)
		return ae.IsAllowed(func(ae *env.AppEnv, rc *response.RequestContext) { r.Patch(ae, rc, params) }, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})

	ae.FabricApi.LinkDeleteLinkHandler = link.DeleteLinkHandlerFunc(func(params link.DeleteLinkParams, _ any) middleware.Responder {
		ae.InitPermissionsContext(params.HTTPRequest, permissions.Management, permissions.Ops, permissions.Delete)
		return ae.IsAllowed(r.Delete, params.HTTPRequest, params.ID, "", permissions.DefaultManagementAccess())
	})
}

func (r *LinkRouter) ListLinks(ae *env.AppEnv, rc *response.RequestContext) {
	ListWithHandler[*model.Link](ae, rc, ae.Managers.Link, MapLinkToRestModel)
}

func (r *LinkRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	DetailWithHandler[*model.Link](ae, rc, ae.Managers.Link, MapLinkToRestModel)
}

func (r *LinkRouter) Patch(ae *env.AppEnv, rc *response.RequestContext, params link.PatchLinkParams) {
	Patch(rc, func(id string, fields fields.UpdatedFields) error {
		n := ae.GetHostController().GetNetwork()
		l, found := n.GetLink(id)
		if !found {
			return boltz.NewNotFoundError("link", "id", id)
		}
		if fields.IsUpdated("staticCost") {
			l.SetStaticCost(int32(params.Link.StaticCost))
		}
		if fields.IsUpdated("down") {
			l.SetDown(params.Link.Down)
		}
		n.RerouteLink(l)
		return nil
	})
}

func (r *LinkRouter) Delete(ae *env.AppEnv, rc *response.RequestContext) {
	Delete(rc, func(rc *response.RequestContext, id string) error {
		return deleteLink(ae.GetHostController().GetNetwork().RemoveLink, id)
	})
}

// deleteLink removes the link with the given id through removeLink, returning a not found error when
// removeLink reports the link was not known. removeLink acts on an unknown id as well, so what it does for one
// happens either way.
func deleteLink(removeLink func(linkId string) bool, id string) error {
	if !removeLink(id) {
		return boltz.NewNotFoundError("link", "id", id)
	}
	return nil
}
