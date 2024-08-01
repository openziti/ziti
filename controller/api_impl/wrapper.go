package api_impl

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/network"
	"net/http"
)

type RequestHandler func(network *network.Network, rc api.RequestContext)

type RequestWrapper interface {
	WrapRequest(handler RequestHandler, request *http.Request, entityId, entitySubId string) middleware.Responder
	WrapHttpHandler(handler http.Handler) http.Handler
	WrapWsHandler(handler http.Handler) http.Handler
}
