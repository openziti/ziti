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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
)

type ackHandler struct {
	link      xlink.Xlink
	forwarder *forwarder.Forwarder
}

func newAckHandler(link xlink.Xlink, forwarder *forwarder.Forwarder) *ackHandler {
	return &ackHandler{
		link:      link,
		forwarder: forwarder,
	}
}

func (self *ackHandler) ContentType() int32 {
	return xgress.ContentTypeAcknowledgementType
}

func (self *ackHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).
		WithField("linkId", self.link.Id().Token).
		WithField("routerId", self.link.DestinationId())

	if ack, err := xgress.UnmarshallChannel2Acknowledgement(msg); err == nil {
		if err := self.forwarder.ForwardAcknowledgement(xgress.Address(self.link.Id().Token), ack); err != nil {
			log.WithError(err).Debug("unable to forward acknowledgement")
		}
	} else {
		log.WithError(err).Error("error unmarshalling ack", err)
	}
}
