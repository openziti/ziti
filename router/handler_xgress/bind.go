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

package handler_xgress

import (
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/metrics"
	"github.com/openziti/fabric/router/xgress"
)

type bindHandler struct {
	receiveHandler     xgress.ReceiveHandler
	closeHandler       xgress.CloseHandler
	metricsPeekHandler xgress.PeekHandler
	forwarder          *forwarder.Forwarder
}

func NewBindHandler(receiveHandler xgress.ReceiveHandler, closeHandler xgress.CloseHandler, forwarder *forwarder.Forwarder) *bindHandler {
	return &bindHandler{
		receiveHandler:     receiveHandler,
		closeHandler:       closeHandler,
		metricsPeekHandler: metrics.NewXgressPeekHandler(forwarder.MetricsRegistry()),
		forwarder:          forwarder,
	}
}

func (bindHandler *bindHandler) HandleXgressBind(x *xgress.Xgress) {
	x.SetReceiveHandler(bindHandler.receiveHandler)
	x.AddPeekHandler(bindHandler.metricsPeekHandler)

	payloadBuffer := bindHandler.forwarder.PayloadBuffer(x)
	x.SetPayloadBuffer(payloadBuffer)

	x.SetCloseHandler(bindHandler.closeHandler)

	bindHandler.forwarder.RegisterDestination(x.SessionId(), x.Address(), x)
}
