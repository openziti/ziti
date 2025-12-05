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
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/websockets"
	"github.com/openziti/edge-api/rest_management_api_server"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/identity"
	"github.com/openziti/xweb/v3"
	"github.com/openziti/ziti/controller/api"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/handler_mgmt"
	"github.com/openziti/ziti/controller/internal/permissions"
	"github.com/openziti/ziti/controller/network"
	"github.com/openziti/ziti/controller/response"
	"github.com/openziti/ziti/controller/rest_client"
	"github.com/openziti/ziti/controller/xmgmt"
)

const (
	ServerHeader = "server"
)

var _ xweb.ApiHandlerFactory = &FabricManagementApiFactory{}

type FabricManagementApiFactory struct {
	InitFunc    func(managementApi *FabricManagementApiHandler) error
	network     *network.Network
	env         *env.AppEnv
	nodeId      identity.Identity
	xmgmts      *concurrenz.CopyOnWriteSlice[xmgmt.Xmgmt]
	MakeDefault bool
}

func (factory *FabricManagementApiFactory) Validate(_ *xweb.InstanceConfig) error {
	return nil
}

func NewFabricManagementApiFactory(nodeId identity.Identity, env *env.AppEnv, network *network.Network, xmgmts *concurrenz.CopyOnWriteSlice[xmgmt.Xmgmt]) *FabricManagementApiFactory {
	pfxlog.Logger().Infof("initializing fabric management api factory with %d xmgmt instances", len(xmgmts.Value()))
	return &FabricManagementApiFactory{
		env:         env,
		network:     network,
		nodeId:      nodeId,
		xmgmts:      xmgmts,
		MakeDefault: false,
	}
}

func (factory *FabricManagementApiFactory) Binding() string {
	return FabricApiBinding
}

func (factory *FabricManagementApiFactory) New(_ *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	managementApiHandler, err := NewFabricManagementApiHandler(factory.env, factory.MakeDefault, options)

	if err != nil {
		return nil, err
	}

	managementApiHandler.bindHandler = handler_mgmt.NewBindHandler(factory.env, factory.network, factory.xmgmts)

	if factory.InitFunc != nil {
		if err := factory.InitFunc(managementApiHandler); err != nil {
			return nil, fmt.Errorf("error running on init func: %v", err)
		}
	}

	return managementApiHandler, nil
}

func NewFabricManagementApiHandler(ae *env.AppEnv, isDefault bool, options map[interface{}]interface{}) (*FabricManagementApiHandler, error) {
	managementApi := &FabricManagementApiHandler{
		options:   options,
		isDefault: isDefault,
		ae:        ae,
	}

	managementApi.handler = managementApi.newHandler()
	managementApi.wsHandler = managementApi.WrapWsHandler(http.HandlerFunc(managementApi.handleWebSocket))
	managementApi.wsUrl = rest_client.DefaultBasePath + "/ws-api"

	return managementApi, nil
}

type FabricManagementApiHandler struct {
	handler     http.Handler
	wsHandler   http.Handler
	wsUrl       string
	options     map[interface{}]interface{}
	bindHandler channel.BindHandler
	isDefault   bool
	ae          *env.AppEnv
}

func (self *FabricManagementApiHandler) Binding() string {
	return FabricApiBinding
}

func (self *FabricManagementApiHandler) Options() map[interface{}]interface{} {
	return self.options
}

func (self *FabricManagementApiHandler) RootPath() string {
	return rest_client.DefaultBasePath
}

func (self *FabricManagementApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, self.RootPath())
}

func (self *FabricManagementApiHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == self.wsUrl {
		self.wsHandler.ServeHTTP(writer, request)
	} else {
		self.handler.ServeHTTP(writer, request)
	}
}

func (self *FabricManagementApiHandler) newHandler() http.Handler {
	innerManagementHandler := self.ae.FabricApi.Serve(nil)
	return self.WrapHttpHandler(innerManagementHandler)
}

func (self *FabricManagementApiHandler) IsDefault() bool {
	return self.isDefault
}

func (self *FabricManagementApiHandler) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
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

	_, err = channel.NewChannel("mgmt", underlayFactory, self.bindHandler, nil)
	if err != nil {
		log.WithError(err).Error("unable to create channel over websocket")
		return
	}
}

func (self *FabricManagementApiHandler) WrapHttpHandler(handler http.Handler) http.Handler {
	wrapped := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set(ZitiInstanceId, self.ae.InstanceId)

		if r.URL.Path == FabricRestApiRootPath {
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

func (self *FabricManagementApiHandler) WrapWsHandler(handler http.Handler) http.Handler {
	wrapped := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rc := self.ae.CreateRequestContext(rw, r)

		err := self.ae.FillRequestContext(rc)
		if err != nil {
			rc.RespondWithError(err)
			return
		}

		if !permissions.IsAdmin().IsAllowed(rc.ActivePermissions...) {
			rc.RespondWithApiError(errorz.NewUnauthorized())
			return
		}

		handler.ServeHTTP(rw, r)
	})

	return wrapped
}
