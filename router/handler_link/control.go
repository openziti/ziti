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

type controlHandler struct {
	link      xlink.Xlink
	forwarder *forwarder.Forwarder
	ctrls     chan *xgress.Control
}

func newControlHandler(link xlink.Xlink, ch channel2.Channel, forwarder *forwarder.Forwarder, closeNotify <-chan struct{}) *controlHandler {
	result := &controlHandler{
		link:      link,
		forwarder: forwarder,
		ctrls:     make(chan *xgress.Control, 4),
	}
	go result.run(ch.Label(), closeNotify)
	return result
}

func (self *controlHandler) ContentType() int32 {
	return xgress.ContentTypeControlType
}

func (self *controlHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	control, err := xgress.UnmarshallControl(msg)
	if err == nil {
		if err := self.forwarder.ForwardControl(xgress.Address(self.link.Id().Token), control); err != nil {
			log.WithError(err).Debug("unable to forward")
		}
	} else {
		log.Errorf("unexpected error (%v)", err)
	}
}

func (self *controlHandler) run(label string, closeNotify <-chan struct{}) {
	log := pfxlog.ContextLogger(label)
	log.Info("starting")
	defer log.Info("exiting")

	for {
		select {
		case control := <-self.ctrls:
			if err := self.forwarder.ForwardControl(xgress.Address(self.link.Id().Token), control); err != nil {
				log.WithError(err).Debug("unable to forward")
			}
		case <-closeNotify:
			return
		}
	}
}
