package server

import (
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_management_api_server"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/api_impl"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/foundation/util/errorz"
	"net/http"
	"time"
)

type fabricWrapper struct {
	ae *env.AppEnv
}

func (self *fabricWrapper) WrapRequest(handler api_impl.RequestHandler, request *http.Request, entityId, entitySubId string) middleware.Responder {
	return middleware.ResponderFunc(func(writer http.ResponseWriter, producer runtime.Producer) {
		rc, err := env.GetRequestContextFromHttpContext(request)

		if rc == nil {
			rc = self.ae.CreateRequestContext(writer, request)
		}

		rc.SetProducer(producer)
		rc.SetEntityId(entityId)
		rc.SetEntitySubId(entitySubId)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not retrieve request context")
			rc.RespondWithError(err)
			return
		}

		if !permissions.IsAdmin().IsAllowed(rc.ActivePermissions...) {
			rc.RespondWithApiError(errorz.NewUnauthorized())
			return
		}

		handler(self.ae.GetHostController().GetNetwork(), rc)
	})
}

func (self *fabricWrapper) WrapHttpHandler(handler http.Handler) http.Handler {
	wrapped := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set(ZitiInstanceId, self.ae.InstanceId)

		if r.URL.Path == api_impl.FabricRestApiRootPath {
			rw.Header().Set("content-type", "application/json")
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write(rest_management_api_server.SwaggerJSON)
			return
		}

		rc := self.ae.CreateRequestContext(rw, r)

		api.AddRequestContextToHttpContext(r, rc)

		err := self.ae.FillRequestContext(rc)
		if err != nil {
			rc.RespondWithError(err)
			return
		}

		//after request context is filled so that api session is present for session expiration headers
		response.AddHeaders(rc)

		handler.ServeHTTP(rw, r)
	})

	return api.TimeoutHandler(api.WrapCorsHandler(wrapped), 10*time.Second, apierror.NewTimeoutError(), response.EdgeResponseMapper{})
}
