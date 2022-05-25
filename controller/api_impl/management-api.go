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
	"crypto/x509"
	"fmt"
	"github.com/go-openapi/loads"
	"github.com/gorilla/websocket"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/channel/websockets"
	"github.com/openziti/fabric/controller/handler_mgmt"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/controller/xmgmt"
	"github.com/openziti/fabric/rest_client"
	"github.com/openziti/fabric/rest_server"
	"github.com/openziti/fabric/rest_server/operations"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/xweb"
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
	nodeId   identity.Identity
	xmgmts   []xmgmt.Xmgmt
}

func (factory *ManagementApiFactory) Validate(_ *xweb.Config) error {
	return nil
}

func NewManagementApiFactory(nodeId identity.Identity, network *network.Network, xmgmts []xmgmt.Xmgmt) *ManagementApiFactory {
	return &ManagementApiFactory{
		network: network,
		nodeId:  nodeId,
		xmgmts:  xmgmts,
	}
}

func (factory *ManagementApiFactory) Binding() string {
	return FabricApiBinding
}

func (factory *ManagementApiFactory) New(_ *xweb.WebListener, options map[interface{}]interface{}) (xweb.WebHandler, error) {
	managementSpec, err := loads.Embedded(rest_server.SwaggerJSON, rest_server.FlatSwaggerJSON)
	if err != nil {
		pfxlog.Logger().Fatalln(err)
	}

	fabricAPI := operations.NewZitiFabricAPI(managementSpec)
	fabricAPI.ServeError = ServeError

	if requestWrapper == nil {
		requestWrapper = &FabricRequestWrapper{
			nodeId:  factory.nodeId,
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

	managementApiHandler.bindHandler = handler_mgmt.NewBindHandler(factory.network, factory.xmgmts)

	if factory.InitFunc != nil {
		if err := factory.InitFunc(managementApiHandler); err != nil {
			return nil, fmt.Errorf("error running on init func: %v", err)
		}
	}

	return managementApiHandler, nil
}

func NewManagementApiHandler(fabricApi *operations.ZitiFabricAPI, options map[interface{}]interface{}) (*ManagementApiHandler, error) {
	managementApi := &ManagementApiHandler{
		fabricApi: fabricApi,
		options:   options,
	}

	managementApi.handler = managementApi.newHandler()
	managementApi.wsHandler = requestWrapper.WrapWsHandler(http.HandlerFunc(managementApi.handleWebSocket))
	managementApi.wsUrl = rest_client.DefaultBasePath + "/ws-api"

	return managementApi, nil
}

type ManagementApiHandler struct {
	fabricApi   *operations.ZitiFabricAPI
	handler     http.Handler
	wsHandler   http.Handler
	wsUrl       string
	options     map[interface{}]interface{}
	bindHandler channel.BindHandler
}

func (managementApi *ManagementApiHandler) Binding() string {
	return FabricApiBinding
}

func (managementApi *ManagementApiHandler) Options() map[interface{}]interface{} {
	return managementApi.options
}

func (managementApi *ManagementApiHandler) RootPath() string {
	return rest_client.DefaultBasePath
}

func (managementApi *ManagementApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, managementApi.RootPath())
}

func (managementApi *ManagementApiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == managementApi.wsUrl {
		managementApi.wsHandler.ServeHTTP(writer, request)
	} else {
		managementApi.handler.ServeHTTP(writer, request)
	}
}

func (managementApi *ManagementApiHandler) newHandler() http.Handler {
	innerManagementHandler := managementApi.fabricApi.Serve(nil)
	return requestWrapper.WrapHttpHandler(innerManagementHandler)
}

func (managementApi *ManagementApiHandler) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
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
