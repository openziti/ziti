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

package handler_mgmt

import (
	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/v2/common/trace"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/openziti/ziti/v2/controller/xmgmt"
)

type BindHandler struct {
	env     *env.AppEnv
	network *network.Network
	xmgmts  *concurrenz.CopyOnWriteSlice[xmgmt.Xmgmt]
}

func NewBindHandler(env *env.AppEnv, network *network.Network, xmgmts *concurrenz.CopyOnWriteSlice[xmgmt.Xmgmt]) channel.BindHandler {
	return &BindHandler{
		env:     env,
		network: network,
		xmgmts:  xmgmts,
	}
}

func (bindHandler *BindHandler) BindChannel(binding channel.Binding) error {
	inspectRequestHandler := newInspectHandler(bindHandler.network)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    inspectRequestHandler.ContentType(),
		Handler: inspectRequestHandler.HandleReceive,
	})

	validateCircuitsRequestHandler := newValidateCircuitsHandler(bindHandler.env)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    validateCircuitsRequestHandler.ContentType(),
		Handler: validateCircuitsRequestHandler.HandleReceive,
	})

	validateTerminatorsRequestHandler := newValidateTerminatorsHandler(bindHandler.network)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    validateTerminatorsRequestHandler.ContentType(),
		Handler: validateTerminatorsRequestHandler.HandleReceive,
	})

	validateLinksRequestHandler := newValidateRouterLinksHandler(bindHandler.network)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    validateLinksRequestHandler.ContentType(),
		Handler: validateLinksRequestHandler.HandleReceive,
	})

	validateGossipRequestHandler := newValidateGossipHandler(bindHandler.network)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    validateGossipRequestHandler.ContentType(),
		Handler: validateGossipRequestHandler.HandleReceive,
	})

	validateSdkTerminatorsRequestHandler := newValidateRouterSdkTerminatorsHandler(bindHandler.network)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    validateSdkTerminatorsRequestHandler.ContentType(),
		Handler: validateSdkTerminatorsRequestHandler.HandleReceive,
	})

	validateIdentityConnectionStatusesRequestHandler := newValidateIdentityConnectionStatusesHandler(bindHandler.env)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    validateIdentityConnectionStatusesRequestHandler.ContentType(),
		Handler: validateIdentityConnectionStatusesRequestHandler.HandleReceive,
	})

	validateRouterDataModelRequestHandler := newValidateRouterDataModelHandler(bindHandler.env)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    validateRouterDataModelRequestHandler.ContentType(),
		Handler: validateRouterDataModelRequestHandler.HandleReceive,
	})

	validateErtTerminatorsRequestHandler := newValidateRouterErtTerminatorsHandler(bindHandler.network)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    validateErtTerminatorsRequestHandler.ContentType(),
		Handler: validateErtTerminatorsRequestHandler.HandleReceive,
	})

	validateControllerDialersRequestHandler := newValidateControllerDialersHandler(bindHandler.network)
	channel.AddReceiveHandlers(binding, &channel.AsyncFunctionReceiveAdapter{
		Type:    validateControllerDialersRequestHandler.ContentType(),
		Handler: validateControllerDialersRequestHandler.HandleReceive,
	})

	tracesHandler := newStreamTracesHandler(bindHandler.network)
	channel.AddReceiveHandlers(binding, tracesHandler)
	binding.AddCloseHandler(tracesHandler)

	eventsHandler := newStreamEventsHandler(bindHandler.network)
	channel.AddReceiveHandlers(binding, eventsHandler)
	binding.AddCloseHandler(eventsHandler)

	channel.AddReceiveHandlers(binding, newTogglePipeTracesHandler(bindHandler.network))

	binding.AddPeekHandler(trace.NewChannelPeekHandler(bindHandler.network.GetAppId(), binding.GetChannel(), bindHandler.network.GetTraceController()))

	xmgmtDone := make(chan struct{})
	for _, x := range bindHandler.xmgmts.Value() {
		if err := x.BindChannel(binding); err != nil {
			return err
		}
		if err := x.Run(binding.GetChannel(), xmgmtDone); err != nil {
			return err
		}
	}
	if len(bindHandler.xmgmts.Value()) > 0 {
		binding.AddCloseHandler(newXmgmtCloseHandler(xmgmtDone))
	}

	return nil
}
