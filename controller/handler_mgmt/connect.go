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

package handler_mgmt

import (
	"crypto/x509"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type ConnectHandler struct {
	network *network.Network
}

func NewConnectHandler(network *network.Network) *ConnectHandler {
	return &ConnectHandler{
		network: network,
	}
}

func (h *ConnectHandler) HandleConnection(hello *channel2.Hello, certificates []*x509.Certificate) error {
	return nil
}

func sendSuccess(request *channel2.Message, ch channel2.Channel, message string) {
	sendResult(request, ch, message, true)
}

func sendFailure(request *channel2.Message, ch channel2.Channel, message string) {
	sendResult(request, ch, message, false)
}

func sendResult(request *channel2.Message, ch channel2.Channel, message string, success bool) {
	log := pfxlog.ContextLogger(ch.Label())
	if !success {
		log.Errorf("mgmt error (%s)", message)
	}

	response := channel2.NewResult(success, message)
	response.ReplyTo(request)
	_ = ch.Send(response)
	log.Debug("success")
}
