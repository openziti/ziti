package api_impl

import (
	"crypto/x509"
	"github.com/go-openapi/runtime"
	openApiMiddleware "github.com/go-openapi/runtime/middleware"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/build"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/rest_server"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/errorz"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

var requestWrapper RequestWrapper

func OverrideRequestWrapper(rw RequestWrapper) {
	if requestWrapper != nil {
		pfxlog.Logger().Warn("requestWrapper overridden more than once")
	}
	requestWrapper = rw
}

type RequestHandler func(network *network.Network, rc api.RequestContext)

type RequestWrapper interface {
	WrapRequest(handler RequestHandler, request *http.Request, entityId, entitySubId string) openApiMiddleware.Responder
	WrapHttpHandler(handler http.Handler) http.Handler
	WrapWsHandler(handler http.Handler) http.Handler
}

type FabricRequestWrapper struct {
	nodeId  identity.Identity
	network *network.Network
}

func (self *FabricRequestWrapper) WrapRequest(handler RequestHandler, request *http.Request, entityId, entitySubId string) openApiMiddleware.Responder {
	return openApiMiddleware.ResponderFunc(func(writer http.ResponseWriter, producer runtime.Producer) {
		rc, err := api.GetRequestContextFromHttpContext(request)

		if rc == nil {
			rc = NewRequestContext(writer, request)
		}

		rc.SetProducer(producer)
		rc.SetEntityId(entityId)
		rc.SetEntitySubId(entitySubId)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not retrieve request context")
			rc.RespondWithError(err)
			return
		}

		handler(self.network, rc)
	})
}

func (self *FabricRequestWrapper) WrapHttpHandler(handler http.Handler) http.Handler {
	wrapper := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == FabricRestApiSpecUrl {
			rw.Header().Set("content-type", "application/json")
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write(rest_server.SwaggerJSON)
			return
		}

		rc := NewRequestContext(rw, r)

		if err := self.verifyCert(r); err != nil {
			rc.RespondWithError(apierror.NewInvalidAuth())
			return
		}

		api.AddRequestContextToHttpContext(r, rc)

		//after request context is filled so that api session is present for session expiration headers
		buildInfo := build.GetBuildInfo()
		if buildInfo != nil {
			rc.GetResponseWriter().Header().Set(ServerHeader, "ziti-controller/"+buildInfo.Version())
		}

		handler.ServeHTTP(rw, r)
	})

	return api.TimeoutHandler(api.WrapCorsHandler(wrapper), 10*time.Second, apierror.NewTimeoutError(), fabricResponseMapper{})
}

func (self *FabricRequestWrapper) WrapWsHandler(handler http.Handler) http.Handler {
	wrapper := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if err := self.verifyCert(r); err != nil {
			rc := NewRequestContext(rw, r)
			rc.RespondWithError(apierror.NewInvalidAuth())
			return
		}

		handler.ServeHTTP(rw, r)
	})

	return wrapper
}

func (self *FabricRequestWrapper) verifyCert(r *http.Request) error {
	certificates := r.TLS.PeerCertificates
	if len(certificates) == 0 {
		return errors.New("no certificates provided, unable to verify dialer")
	}

	config := self.nodeId.ServerTLSConfig()

	opts := x509.VerifyOptions{
		Roots:         config.RootCAs,
		Intermediates: x509.NewCertPool(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	var errorList errorz.MultipleErrors

	for _, cert := range certificates {
		if _, err := cert.Verify(opts); err == nil {
			return nil
		} else {
			errorList = append(errorList, err)
		}
	}

	//goland:noinspection GoNilness
	return errorList.ToError()
}
