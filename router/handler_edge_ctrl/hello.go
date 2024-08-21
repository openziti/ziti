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

package handler_edge_ctrl

import (
	"strconv"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/ziti/common/build"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/router/state"
	"google.golang.org/protobuf/proto"
)

type helloHandler struct {
	listeners []*edge_ctrl_pb.Listener

	//backwards compat for controllers v0.26.3 and older
	hostname           string
	supportedProtocols []string
	protocolPorts      []string
	stateManager       state.Manager
}

func NewHelloHandler(stateManager state.Manager, listeners []*edge_ctrl_pb.Listener) *helloHandler {
	//supportedProtocols, protocolPorts, and hostname is for backwards compatibility with v0.26.3 and older controllers
	var supportedProtocols []string
	var protocolPorts []string
	hostname := ""

	for _, listener := range listeners {
		pfxlog.Logger().Debugf("HelloHandler will contain supportedProtocols address: %s advertise: %s", listener.Address.Value, listener.Advertise.Value)

		supportedProtocols = append(supportedProtocols, listener.Address.Protocol)
		protocolPorts = append(protocolPorts, strconv.Itoa(int(listener.Advertise.Port)))

		if hostname != "" && hostname != listener.Advertise.Hostname {
			pfxlog.Logger().Warnf("this router is configured to use different hostnames for different edge listeners. If the controller is v0.26.3 or earlier this is not supported. Advertise %s will be used for all protocols", listeners[0].Advertise.Value)
		}

		hostname = listener.Advertise.Hostname
	}

	return &helloHandler{
		listeners:    listeners,
		stateManager: stateManager,

		//v0.26.3 and older used to check and ensure all advertise hostnames were the same which can't be done now
		//with the ability to report multiple advertise protocols on different hostnames
		hostname:           hostname,
		supportedProtocols: supportedProtocols,
		protocolPorts:      protocolPorts,
	}
}

func (h *helloHandler) ContentType() int32 {
	return env.ServerHelloType
}

func (h *helloHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	go func() {

		serverHello := &edge_ctrl_pb.ServerHello{}
		if err := proto.Unmarshal(msg.Body, serverHello); err == nil {
			pfxlog.Logger().Info("received server hello, replying")

			clientHello := &edge_ctrl_pb.ClientHello{
				Version:       build.GetBuildInfo().Version(),
				Listeners:     h.listeners,
				Hostname:      h.hostname,
				Protocols:     h.supportedProtocols,
				ProtocolPorts: h.protocolPorts,
				Data:          map[string]string{},
			}

			outMsg := protobufs.MarshalTyped(clientHello).ToSendable().Msg()

			if h.stateManager.GetEnv().IsHaEnabled() {
				if supported, ok := msg.Headers.GetBoolHeader(int32(edge_ctrl_pb.Header_RouterDataModel)); ok && supported {

					outMsg.Headers.PutBoolHeader(int32(edge_ctrl_pb.Header_RouterDataModel), true)

					if index, ok := h.stateManager.RouterDataModel().CurrentIndex(); ok {
						outMsg.Headers.PutUint64Header(int32(edge_ctrl_pb.Header_RouterDataModelIndex), index)
					}
				}
			}

			if err := outMsg.ReplyTo(msg).Send(ch); err != nil {
				pfxlog.Logger().WithError(err).Error("could not send client hello")
			}
			return
		} else {
			pfxlog.Logger().WithError(err).Error("could not unmarshal server hello")
		}
	}()
}
