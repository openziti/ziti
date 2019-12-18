/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/gateway/internal/fabric"
	"github.com/netfoundry/ziti-edge/pb/edge_ctrl_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/common/version"
)

type helloHandler struct {
	sm                 fabric.StateManager
	supportedProtocols []string
	hostname           string
}

func NewHelloHandler(hostname string, supportedProtocols []string) *helloHandler {
	return &helloHandler{
		hostname:           hostname,
		supportedProtocols: supportedProtocols,
	}
}

func (h *helloHandler) ContentType() int32 {
	return env.ServerHelloType
}

func (h *helloHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	go func() {
		serverHello := &edge_ctrl_pb.ServerHello{}
		if err := proto.Unmarshal(msg.Body, serverHello); err == nil {
			pfxlog.Logger().Info("received server hello, replying")

			clientHello := &edge_ctrl_pb.ClientHello{
				Version:   version.GetVersion(),
				Hostname:  h.hostname,
				Protocols: h.supportedProtocols,
			}

			clientHelloBuff, err := proto.Marshal(clientHello)

			if err != nil {
				pfxlog.Logger().WithField("cause", err).Error("could not marshal client hello")
				return
			}
			clientHelloMsg := channel2.NewMessage(env.ClientHelloType, clientHelloBuff)
			clientHelloMsg.ReplyTo(msg)
			ch.Send(clientHelloMsg)
			return
		} else {
			pfxlog.Logger().WithField("cause", err).Error("could not unmarshal server hello")
		}
	}()
}
