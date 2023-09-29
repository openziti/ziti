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

package handler_link

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
)

type payloadHandler struct {
	link      xlink.Xlink
	forwarder *forwarder.Forwarder
}

func newPayloadHandler(link xlink.Xlink, forwarder *forwarder.Forwarder) *payloadHandler {
	return &payloadHandler{
		link:      link,
		forwarder: forwarder,
	}
}

func (self *payloadHandler) ContentType() int32 {
	return xgress.ContentTypePayloadType
}

func (self *payloadHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).
		WithField("linkId", self.link.Id()).
		WithField("routerId", self.link.DestinationId())

	payload, err := xgress.UnmarshallPayload(msg)
	if err == nil {
		if err = self.forwarder.ForwardPayload(xgress.Address(self.link.Id()), payload); err != nil {
			log.WithError(err).Debug("unable to forward")
			self.forwarder.ReportForwardingFault(payload.CircuitId, "")
		}
		if payload.IsCircuitEndFlagSet() {
			self.forwarder.EndCircuit(payload.GetCircuitId())
		}
	} else {
		log.WithError(err).Errorf("error unmarshalling payload")
	}
}
