package routes

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_server/operations/current_api_session"
)

func init() {
	r := NewCurrentIdentityRouter()
	env.AddRouter(r)
}

type CurrentIdentityRouter struct {
	BasePath string
}

func NewCurrentIdentityRouter() *CurrentIdentityRouter {
	return &CurrentIdentityRouter{
		BasePath: "/" + EntityNameCurrentIdentity,
	}
}

func (r *CurrentIdentityRouter) Register(ae *env.AppEnv) {
	// current identity
	ae.Api.CurrentAPISessionGetCurrentIdentityHandler = current_api_session.GetCurrentIdentityHandlerFunc(func(params current_api_session.GetCurrentIdentityParams, _ interface{}) middleware.Responder {
		return ae.IsAllowed(detailCurrentUser, params.HTTPRequest, "", "", permissions.IsAuthenticated())
	})
}

func detailCurrentUser(ae *env.AppEnv, rc *response.RequestContext) {
	result, err := MapIdentityToRestModel(ae, rc.Identity)

	if err != nil {
		rc.RespondWithError(err)
		return
	}

	result.BaseEntity.Links = CurrentIdentityLinkFactory.Links(rc.Identity)

	rc.RespondWithOk(result, nil)
}
