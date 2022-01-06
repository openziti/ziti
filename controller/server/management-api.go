/*
	Copyright NetFoundry, Inc.

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

package server

import (
	"fmt"
	"github.com/openziti/edge/controller"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_management_api_client"
	"github.com/openziti/edge/rest_management_api_server"
	"github.com/openziti/fabric/controller/api"
	"github.com/openziti/fabric/xweb"
	"net/http"
	"strings"
	"time"
)

var _ xweb.WebHandlerFactory = &ManagementApiFactory{}

type ManagementApiFactory struct {
	InitFunc func(managementApi *ManagementApiHandler) error
	appEnv   *env.AppEnv
}

func (factory ManagementApiFactory) Validate(_ *xweb.Config) error {
	return nil
}

func NewManagementApiFactory(appEnv *env.AppEnv) *ManagementApiFactory {
	return &ManagementApiFactory{
		appEnv: appEnv,
	}
}

func (factory ManagementApiFactory) Binding() string {
	return controller.ManagementApiBinding
}

func (factory ManagementApiFactory) New(_ *xweb.WebListener, options map[interface{}]interface{}) (xweb.WebHandler, error) {
	managementApi, err := NewManagementApiHandler(factory.appEnv, options)

	if err != nil {
		return nil, err
	}

	if factory.InitFunc != nil {
		if err := factory.InitFunc(managementApi); err != nil {
			return nil, fmt.Errorf("error running on init func: %v", err)
		}
	}

	return managementApi, nil
}

type ManagementApiHandler struct {
	handler http.Handler
	appEnv  *env.AppEnv
	options map[interface{}]interface{}
}

func (managementApi ManagementApiHandler) Binding() string {
	return controller.ManagementApiBinding
}

func (managementApi ManagementApiHandler) Options() map[interface{}]interface{} {
	return managementApi.options
}

func (managementApi ManagementApiHandler) RootPath() string {
	return rest_management_api_client.DefaultBasePath
}

func (managementApi ManagementApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, managementApi.RootPath())
}

func (managementApi ManagementApiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	managementApi.handler.ServeHTTP(writer, request)
}

func NewManagementApiHandler(ae *env.AppEnv, options map[interface{}]interface{}) (*ManagementApiHandler, error) {
	managementApi := &ManagementApiHandler{
		options: options,
		appEnv:  ae,
	}

	managementApi.handler = managementApi.newHandler(ae)

	return managementApi, nil
}
func (managementApi ManagementApiHandler) newHandler(ae *env.AppEnv) http.Handler {
	innerManagementHandler := ae.ManagementApi.Serve(nil)

	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw.Header().Set(ZitiInstanceId, ae.InstanceId)

		if r.URL.Path == controller.ManagementRestApiSpecUrl {
			rw.Header().Set("content-type", "application/json")
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write(rest_management_api_server.SwaggerJSON)
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

		innerManagementHandler.ServeHTTP(rw, r)
		timer := ae.GetHostController().GetNetwork().GetMetricsRegistry().Timer(getMetricTimerName(r))
		timer.UpdateSince(start)
	})

	return api.TimeoutHandler(api.WrapCorsHandler(handler), 10*time.Second, apierror.NewTimeoutError(), response.EdgeResponseMapper{})
}

// getMetricTimerName returns a metric timer name based on the incoming HTTP request's URL and method.
// Unique ids are removed from the URL and replaced with :id and :subid to group metrics from the same
// endpoint that happen to be working on different ids.
func getMetricTimerName(r *http.Request) string {
	cleanUrl := r.URL.Path

	rc, _ := api.GetRequestContextFromHttpContext(r)

	if rc != nil {
		if id, err := rc.GetEntityId(); err == nil && id != "" {
			cleanUrl = strings.Replace(cleanUrl, id, ":id", -1)
		}

		if subid, err := rc.GetEntitySubId(); err == nil && subid != "" {
			cleanUrl = strings.Replace(cleanUrl, subid, ":subid", -1)
		}
	}

	return fmt.Sprintf("%s.%s", cleanUrl, r.Method)
}
