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
	"github.com/openziti/channel"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
)

type controlHandler struct {
	link      xlink.Xlink
	forwarder *forwarder.Forwarder
}

func newControlHandler(link xlink.Xlink, forwarder *forwarder.Forwarder) *controlHandler {
	result := &controlHandler{
		link:      link,
		forwarder: forwarder,
	}
	return result
}

func (self *controlHandler) ContentType() int32 {
	return xgress.ContentTypeControlType
}

func (self *controlHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	if control, err := xgress.UnmarshallControl(msg); err == nil {
		if err = self.forwarder.ForwardControl(xgress.Address(self.link.Id().Token), control); err != nil {
			log.WithError(err).Debug("unable to forward")
		}
	} else {
		log.WithError(err).Errorf("unexpected error marshalling control instance")
	}
}
