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

package api_impl

import (
	"fmt"
	"github.com/go-openapi/loads"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/rest_client"
	"github.com/openziti/fabric/rest_server"
	"github.com/openziti/fabric/rest_server/operations"
	"github.com/openziti/fabric/xweb"
	"github.com/openziti/foundation/identity/identity"
	"net/http"
	"strings"
)

const (
	ServerHeader = "server"
)

var _ xweb.WebHandlerFactory = &ManagementApiFactory{}

type ManagementApiFactory struct {
	InitFunc func(managementApi *ManagementApiHandler) error
	network  *network.Network
	routerId identity.Identity
}

func (factory ManagementApiFactory) Validate(_ *xweb.Config) error {
	return nil
}

func NewManagementApiFactory(routerId identity.Identity, network *network.Network) *ManagementApiFactory {
	return &ManagementApiFactory{
		network:  network,
		routerId: routerId,
	}
}

func (factory ManagementApiFactory) Binding() string {
	return FabricApiBinding
}

func (factory ManagementApiFactory) New(_ *xweb.WebListener, options map[interface{}]interface{}) (xweb.WebHandler, error) {
	managementSpec, err := loads.Embedded(rest_server.SwaggerJSON, rest_server.FlatSwaggerJSON)
	if err != nil {
		pfxlog.Logger().Fatalln(err)
	}

	fabricAPI := operations.NewZitiFabricAPI(managementSpec)
	fabricAPI.ServeError = ServeError

	if requestWrapper == nil {
		requestWrapper = &FabricRequestWrapper{
			network: factory.network,
		}
	}

	for _, router := range routers {
		router.Register(fabricAPI, requestWrapper)
	}

	managementApiHandler, err := NewManagementApiHandler(fabricAPI, options)

	if err != nil {
		return nil, err
	}

	if factory.InitFunc != nil {
		if err := factory.InitFunc(managementApiHandler); err != nil {
			return nil, fmt.Errorf("error running on init func: %v", err)
		}
	}

	return managementApiHandler, nil
}

type ManagementApiHandler struct {
	fabricApi *operations.ZitiFabricAPI
	handler   http.Handler
	options   map[interface{}]interface{}
}

func (managementApi ManagementApiHandler) Binding() string {
	return FabricApiBinding
}

func (managementApi ManagementApiHandler) Options() map[interface{}]interface{} {
	return managementApi.options
}

func (managementApi ManagementApiHandler) RootPath() string {
	return rest_client.DefaultBasePath
}

func (managementApi ManagementApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, managementApi.RootPath())
}

func (managementApi ManagementApiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	managementApi.handler.ServeHTTP(writer, request)
}

func NewManagementApiHandler(fabricApi *operations.ZitiFabricAPI, options map[interface{}]interface{}) (*ManagementApiHandler, error) {
	managementApi := &ManagementApiHandler{
		fabricApi: fabricApi,
		options:   options,
	}

	managementApi.handler = managementApi.newHandler()

	return managementApi, nil
}

func (managementApi ManagementApiHandler) newHandler() http.Handler {
	innerManagementHandler := managementApi.fabricApi.Serve(nil)
	return requestWrapper.WrapHttpHandler(innerManagementHandler)
}
