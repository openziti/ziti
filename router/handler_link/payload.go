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
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type payloadHandler struct {
	link      *forwarder.Link
	ctrl      xgress.CtrlChannel
	forwarder *forwarder.Forwarder
}

func newPayloadHandler(link *forwarder.Link, ctrl xgress.CtrlChannel, forwarder *forwarder.Forwarder) *payloadHandler {
	return &payloadHandler{link: link, ctrl: ctrl, forwarder: forwarder}
}

func (h *payloadHandler) ContentType() int32 {
	return xgress.ContentTypePayloadType
}

func (h *payloadHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	payload, err := xgress.UnmarshallPayload(msg)
	if err == nil {
		if err := h.forwarder.ForwardPayload(xgress.Address(h.link.Id.Token), payload); err != nil {
			log.Debugf("unable to forward (%s)", err)
		}
		if payload.IsSessionEndFlagSet() {
			if err := h.forwarder.EndSession(&identity.TokenId{Token: payload.GetSessionId()}); err != nil {
				pfxlog.ContextLogger(ch.Label()).Errorf("failed to end session [s/%s] (%s)", payload.GetSessionId(), err)
			}
		}
	} else {
		log.Errorf("unexpected error (%s)", err)
	}
}
