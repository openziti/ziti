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

package handler_edge_ctrl

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/fabric/build"
)

type helloHandler struct {
	supportedProtocols []string
	hostname           string
	protocolPorts      []string
}

func NewHelloHandler(hostname string, supportedProtocols []string, protocolPorts []string) *helloHandler {
	return &helloHandler{
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
				Hostname:      h.hostname,
				Protocols:     h.supportedProtocols,
				ProtocolPorts: h.protocolPorts,
			}
			if err := protobufs.MarshalTyped(clientHello).ReplyTo(msg).Send(ch); err != nil {
				pfxlog.Logger().WithError(err).Error("could not send client hello")
			}
			return
		} else {
			pfxlog.Logger().WithError(err).Error("could not unmarshal server hello")
		}
	}()
}
