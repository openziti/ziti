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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xgress"
)

type receiveHandler struct {
	forwarder *forwarder.Forwarder
}

func NewReceiveHandler(forwarder *forwarder.Forwarder) *receiveHandler {
	return &receiveHandler{forwarder: forwarder}
}

func (xrh *receiveHandler) HandleXgressReceive(payload *xgress.Payload, x *xgress.Xgress) {
	if err := xrh.forwarder.ForwardPayload(x.Address(), payload); err != nil {
		pfxlog.ContextLogger(x.Label()).WithFields(payload.GetLoggerFields()).Errorf("unable to forward (%s)", err)
	}
}
