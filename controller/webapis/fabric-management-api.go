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
	"crypto/x509"
	"fmt"
	"github.com/go-openapi/loads"
	"github.com/gorilla/websocket"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/websockets"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
	"github.com/openziti/xweb/v2"
	"github.com/openziti/ziti/controller/api_impl"
	"github.com/openziti/ziti/controller/handler_mgmt"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/rest_client"
	"github.com/openziti/ziti/controller/rest_server"
	"github.com/openziti/ziti/controller/rest_server/operations"
	"github.com/openziti/ziti/controller/xmgmt"
	"net/http"
	"strings"
)

const (
	ServerHeader = "server"
)

var _ xweb.ApiHandlerFactory = &FabricManagementApiFactory{}

type FabricManagementApiFactory struct {
	InitFunc func(managementApi *FabricManagementApiHandler) error
	network  *network.Network
	nodeId   identity.Identity
	xmgmts   *concurrenz.CopyOnWriteSlice[xmgmt.Xmgmt]
}

func (factory *FabricManagementApiFactory) Validate(_ *xweb.InstanceConfig) error {
	return nil
}

func NewFabricManagementApiFactory(nodeId identity.Identity, network *network.Network, xmgmts *concurrenz.CopyOnWriteSlice[xmgmt.Xmgmt]) *FabricManagementApiFactory {
	pfxlog.Logger().Infof("initializing management api factory with %d xmgmt instances", len(xmgmts.Value()))
	return &FabricManagementApiFactory{
		network: network,
		nodeId:  nodeId,
		xmgmts:  xmgmts,
	}
}

func (factory *FabricManagementApiFactory) Binding() string {
	return api_impl.FabricApiBinding
}

func (factory *FabricManagementApiFactory) New(_ *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	managementSpec, err := loads.Embedded(rest_server.SwaggerJSON, rest_server.FlatSwaggerJSON)
	if err != nil {
		pfxlog.Logger().Fatalln(err)
	}

	fabricAPI := operations.NewZitiFabricAPI(managementSpec)
	fabricAPI.ServeError = api_impl.ServeError

	if requestWrapper == nil {
		requestWrapper = &FabricRequestWrapper{
			nodeId:  factory.nodeId,
			network: factory.network,
		}
	}

	for _, router := range api_impl.Routers {
		router.Register(fabricAPI, requestWrapper)
	}

	managementApiHandler, err := NewFabricManagementApiHandler(fabricAPI, options)

	if err != nil {
		return nil, err
	}

	managementApiHandler.bindHandler = handler_mgmt.NewBindHandler(factory.network, factory.xmgmts)

	if factory.InitFunc != nil {
		if err := factory.InitFunc(managementApiHandler); err != nil {
			return nil, fmt.Errorf("error running on init func: %v", err)
		}
	}

	return managementApiHandler, nil
}

func NewFabricManagementApiHandler(fabricApi *operations.ZitiFabricAPI, options map[interface{}]interface{}) (*FabricManagementApiHandler, error) {
	managementApi := &FabricManagementApiHandler{
		fabricApi: fabricApi,
		options:   options,
	}

	managementApi.handler = managementApi.newHandler()
	managementApi.wsHandler = requestWrapper.WrapWsHandler(http.HandlerFunc(managementApi.handleWebSocket))
	managementApi.wsUrl = rest_client.DefaultBasePath + "/ws-api"

	return managementApi, nil
}

type FabricManagementApiHandler struct {
	fabricApi   *operations.ZitiFabricAPI
	handler     http.Handler
	wsHandler   http.Handler
	wsUrl       string
	options     map[interface{}]interface{}
	bindHandler channel.BindHandler
}

func (managementApi *FabricManagementApiHandler) Binding() string {
	return api_impl.FabricApiBinding
}

func (managementApi *FabricManagementApiHandler) Options() map[interface{}]interface{} {
	return managementApi.options
}

func (managementApi *FabricManagementApiHandler) RootPath() string {
	return rest_client.DefaultBasePath
}

func (managementApi *FabricManagementApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, managementApi.RootPath())
}

func (managementApi *FabricManagementApiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == managementApi.wsUrl {
		managementApi.wsHandler.ServeHTTP(writer, request)
	} else {
		managementApi.handler.ServeHTTP(writer, request)
	}
}

func (managementApi *FabricManagementApiHandler) newHandler() http.Handler {
	innerManagementHandler := managementApi.fabricApi.Serve(nil)
	return requestWrapper.WrapHttpHandler(innerManagementHandler)
}

func (managementApi *FabricManagementApiHandler) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	log := pfxlog.Logger()
	log.Debug("handling mgmt channel websocket upgrade")
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(writer, request, nil)
	if err != nil {
		log.WithError(err).Error("unable to upgrade request to websocket")
		return
	}

	var certs []*x509.Certificate
	if request.TLS != nil {
		certs = request.TLS.PeerCertificates
	}

	id := &identity.TokenId{Token: "mgmt"}
	underlayFactory := websockets.NewUnderlayFactory(id, conn, certs)

	_, err = channel.NewChannel("mgmt", underlayFactory, managementApi.bindHandler, nil)
	if err != nil {
		log.WithError(err).Error("unable to create channel over websocket")
		return
	}
}
