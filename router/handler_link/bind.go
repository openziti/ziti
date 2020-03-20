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

package handler_link

import (
	"github.com/netfoundry/ziti-fabric/metrics"
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-fabric/trace"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type bindHandler struct {
	id        *identity.TokenId
	link      *forwarder.Link
	ctrl      xgress.CtrlChannel
	forwarder *forwarder.Forwarder
}

func NewBindHandler(id *identity.TokenId, link *forwarder.Link, ctrl xgress.CtrlChannel, forwarder *forwarder.Forwarder) *bindHandler {
	return &bindHandler{id: id, link: link, ctrl: ctrl, forwarder: forwarder}
}

func (bindHandler *bindHandler) BindChannel(ch channel2.Channel) error {
	ch.SetLogicalName("l/" + bindHandler.link.Id.Token)
	ch.SetUserData(bindHandler.link.Id.Token)
	ch.AddCloseHandler(newCloseHandler(bindHandler.link, bindHandler.ctrl, bindHandler.forwarder))
	ch.AddErrorHandler(newErrorHandler(bindHandler.link, bindHandler.ctrl))
	ch.AddReceiveHandler(newPayloadHandler(bindHandler.link, bindHandler.ctrl, bindHandler.forwarder))
	ch.AddReceiveHandler(newAckHandler(bindHandler.link, bindHandler.ctrl, bindHandler.forwarder))
	ch.AddReceiveHandler(&channel2.LatencyHandler{})
	ch.AddPeekHandler(metrics.NewChannelPeekHandler(bindHandler.link.Id.Token, bindHandler.forwarder.MetricsRegistry()))
	ch.AddPeekHandler(trace.NewChannelPeekHandler(bindHandler.link.Id, ch, bindHandler.forwarder.TraceController(), trace.NewChannelSink(bindHandler.ctrl.Channel())))
	return nil
}
