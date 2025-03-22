package webapis

import (
	"crypto/x509"
	"errors"
	"github.com/go-openapi/runtime"
	openApiMiddleware "github.com/go-openapi/runtime/middleware"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/common/build"
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/api_impl"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/rest_server"
	"net/http"
	"time"
)

var requestWrapper api_impl.RequestWrapper

func OverrideRequestWrapper(rw api_impl.RequestWrapper) {
	if requestWrapper != nil {
		pfxlog.Logger().Warn("requestWrapper overridden more than once")
	}
	requestWrapper = rw
}

type FabricRequestWrapper struct {
	nodeId  identity.Identity
	network *network.Network
}

func (self *FabricRequestWrapper) WrapRequest(handler api_impl.RequestHandler, request *http.Request, entityId, entitySubId string) openApiMiddleware.Responder {
	return openApiMiddleware.ResponderFunc(func(writer http.ResponseWriter, producer runtime.Producer) {
		rc, err := api.GetRequestContextFromHttpContext(request)

		if rc == nil {
			rc = api_impl.NewRequestContext(writer, request)
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
		if r.URL.Path == api_impl.FabricRestApiSpecUrl {
			rw.Header().Set("content-type", "application/json")
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write(rest_server.SwaggerJSON)
			return
		}

		rc := api_impl.NewRequestContext(rw, r)

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

	return api.TimeoutHandler(api.WrapCorsHandler(wrapper), 10*time.Second, apierror.NewTimeoutError(), api_impl.FabricResponseMapper{})
}

func (self *FabricRequestWrapper) WrapWsHandler(handler http.Handler) http.Handler {
	wrapper := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if err := self.verifyCert(r); err != nil {
			rc := api_impl.NewRequestContext(rw, r)
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

	var errorList []error

	for _, cert := range certificates {
		if _, err := cert.Verify(opts); err == nil {
			return nil
		} else {
			errorList = append(errorList, err)
		}
	}

	return errors.Join(errorList...)
}
