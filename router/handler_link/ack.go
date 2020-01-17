/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type ackHandler struct {
	link      *forwarder.Link
	ctrl      xgress.CtrlChannel
	forwarder *forwarder.Forwarder
}

func newAckHandler(link *forwarder.Link, ctrl xgress.CtrlChannel, forwarder *forwarder.Forwarder) *ackHandler {
	return &ackHandler{link: link, ctrl: ctrl, forwarder: forwarder}
}

func (ackHandler *ackHandler) ContentType() int32 {
	return xgress.ContentTypeAcknowledgementType
}

func (ackHandler *ackHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	if ack, err := xgress.UnmarshallAcknowledgement(msg); err == nil {
		if err := ackHandler.forwarder.ForwardAcknowledgement(xgress.Address(ackHandler.link.Id.Token), ack); err != nil {
			log.Debugf("unable to forward acknowledgement (%s)", err)
		}
	} else {
		log.Errorf("unexpected error (%s)", err)
	}
}
