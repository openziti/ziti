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

package webapis

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/openziti/edge-api/rest_client_api_client"
	"github.com/openziti/edge-api/rest_client_api_server"
	"github.com/openziti/edge-api/rest_management_api_server"
	"github.com/openziti/xweb/v3"
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/response"
	"github.com/pkg/errors"
)

const ZitiInstanceId = "ziti-instance-id"

var _ xweb.ApiHandlerFactory = &ClientApiFactory{}

type ClientApiFactory struct {
	InitFunc func(clientApi *ClientApiHandler) error
	appEnv   *env.AppEnv
}

func (factory ClientApiFactory) Validate(config *xweb.InstanceConfig) error {
	clientApiFound := false
	edgeConfig := factory.appEnv.GetConfig().Edge
	for _, webListener := range config.ServerConfigs {
		for _, api := range webListener.APIs {

			if webListener.Identity != nil && (api.Binding() == ClientApiBinding || api.Binding() == ManagementApiBinding) {
				caBytes, err := os.ReadFile(webListener.Identity.GetConfig().CA)

				if err != nil {
					return errors.Errorf("could not read xweb web listener [%s]'s CA file [%s] to retrieve CA PEMs: %v", webListener.Name, webListener.Identity.GetConfig().CA, err)
				}

				edgeConfig.AddCaPems(caBytes)
			}

			if !clientApiFound && api.Binding() == ClientApiBinding {
				for _, bindPoint := range webListener.BindPoints {
					if bindPoint.ServerAddress() == edgeConfig.Api.Address {
						factory.appEnv.SetClientApiDefaultCertificate(webListener.Identity.ServerCert()[0])
						clientApiFound = true
						break
					}
				}
			}
		}
	}

	edgeConfig.RefreshCas()

	if !clientApiFound {
		return errors.Errorf("could not find [edge.api.address] value [%s] as a bind point any instance of ApiConfig [%s]", edgeConfig.Api.Address, ClientApiBinding)
	}

	return nil
}

func NewClientApiFactory(appEnv *env.AppEnv) *ClientApiFactory {
	return &ClientApiFactory{
		appEnv: appEnv,
	}
}

func (factory ClientApiFactory) Binding() string {
	return ClientApiBinding
}

func (factory ClientApiFactory) New(_ *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	clientApi, err := NewClientApiHandler(factory.appEnv, options)

	if err != nil {
		return nil, err
	}

	if factory.InitFunc != nil {
		if err := factory.InitFunc(clientApi); err != nil {
			return nil, fmt.Errorf("error running on init func: %v", err)
		}
	}

	return clientApi, nil
}

type ClientApiHandler struct {
	handler http.Handler
	appEnv  *env.AppEnv
	options map[interface{}]interface{}
}

func (clientApi ClientApiHandler) Binding() string {
	return ClientApiBinding
}

func (clientApi ClientApiHandler) Options() map[interface{}]interface{} {
	return clientApi.options
}

func (clientApi ClientApiHandler) RootPath() string {
	return rest_client_api_client.DefaultBasePath
}

func (clientApi ClientApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, clientApi.RootPath()) || r.URL.Path == WellKnownEstCaCerts || r.URL.Path == VersionPath || r.URL.Path == RootPath
}

func (clientApi ClientApiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	clientApi.handler.ServeHTTP(writer, request)
}

func (clientApi ClientApiHandler) IsDefault() bool {
	return true
}

func NewClientApiHandler(ae *env.AppEnv, options map[interface{}]interface{}) (*ClientApiHandler, error) {
	clientApi := &ClientApiHandler{
		options: options,
		appEnv:  ae,
	}

	clientApi.handler = clientApi.newHandler(ae)

	return clientApi, nil
}

func (clientApi ClientApiHandler) newHandler(ae *env.AppEnv) http.Handler {
	innerClientHandler := ae.ClientApi.Serve(nil)

	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set(ZitiInstanceId, ae.InstanceId)

		//if not /edge prefix and not /fabric, translate to "/edge/client/v<latest>", this is a hack
		//that should be removed once non-prefixed URLs are no longer used.
		//This will affect older go-lang enrolled SDKs and the C-SDK.
		if !strings.HasPrefix(r.URL.Path, RestApiRootPath) && !strings.HasPrefix(r.URL.Path, "/fabric") && !strings.HasPrefix(r.URL.Path, "/.well-known") {
			r.URL.Path = ClientRestApiBaseUrlLatest + r.URL.Path
		}

		//translate /edge/v1 to /edge/client/v1
		r.URL.Path = strings.Replace(r.URL.Path, LegacyClientRestApiBaseUrlV1, ClientRestApiBaseUrlLatest, 1)

		// .well-known/est/cacerts can be handled by the client API but the generated server requires
		// the prefixed path for route resolution.
		if r.URL.Path == WellKnownEstCaCerts {
			r.URL.Path = ClientRestApiBaseUrlLatest + WellKnownEstCaCerts
		}

		if r.URL.Path == VersionPath || r.URL.Path == RootPath {
			r.URL.Path = ClientRestApiBaseUrlLatest + VersionPath
		}

		if r.URL.Path == ClientRestApiSpecUrl {

			//work around for: https://github.com/go-openapi/runtime/issues/226
			if referer := r.Header.Get("Referer"); referer != "" {
				if strings.Contains(referer, ManagementRestApiBaseUrlLatest) {
					rw.Header().Set("content-type", "application/json")
					rw.WriteHeader(http.StatusOK)
					_, _ = rw.Write(rest_management_api_server.SwaggerJSON)
					return
				}
			}

			rw.Header().Set("content-type", "application/json")
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write(rest_client_api_server.SwaggerJSON)
			return
		}

		rc := ae.CreateRequestContext(rw, r)

		api.AddRequestContextToHttpContext(r, rc)

		err := ae.FillRequestContext(rc)
		if err != nil {
			rc.RespondWithError(err)
			return
		}

		//after request context is filled so that api session is present for session expiration headers
		response.AddHeaders(rc)

		innerClientHandler.ServeHTTP(rw, r)
	})

	return api.TimeoutHandler(api.WrapCorsHandler(handler), 10*time.Second, apierror.NewTimeoutError(), response.EdgeResponseMapper{})
}
