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

package handler_ctrl

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type dialHandler struct {
	id      *identity.TokenId
	ctrl    xgress.CtrlChannel
	dialers []xlink.Dialer
	pool    handlerPool
}

func newDialHandler(id *identity.TokenId, ctrl xgress.CtrlChannel, dialers []xlink.Dialer, forwarder *forwarder.Forwarder, closeNotify chan struct{}) *dialHandler {
	handler := &dialHandler{
		id:      id,
		ctrl:    ctrl,
		dialers: dialers,
		pool: handlerPool{
			options:     forwarder.Options.LinkDial,
			closeNotify: closeNotify,
		},
	}
	handler.pool.Start()

	return handler
}

func (self *dialHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_DialType)
}

func (self *dialHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	self.pool.Queue(func() {
		self.handle(msg, ch)
	})
}

func (self *dialHandler) handle(msg *channel2.Message, _ channel2.Channel) {
	logrus.Info("received link connect request")
	dial := &ctrl_pb.Dial{}
	if err := proto.Unmarshal(msg.Body, dial); err == nil {
		log := pfxlog.ChannelLogger("link", "linkDialer").
			WithFields(logrus.Fields{"linkId": dial.LinkId, "routerId": dial.RouterId, "address": dial.Address})

		linkId := self.id.ShallowCloneWithNewToken(dial.LinkId)
		if len(self.dialers) == 1 {
			log.Info("dialing link")
			if err := self.dialers[0].Dial(dial.Address, linkId, self.id.Token); err == nil {
				if err := self.sendLinkMessage(dial.LinkId); err != nil {
					log.WithError(err).Error("error sending link message ")
				}
				log.Info("link established")
			} else {
				log.WithError(err).Error("link dialing failed")
				if err := self.sendLinkFault(dial.LinkId); err != nil {
					log.WithError(err).Error("error sending fault")
				}
			}
		} else {
			log.Errorf("invalid Xlink dialers configuration")
			if err := self.sendLinkFault(dial.LinkId); err != nil {
				log.WithError(err).Error("error sending link fault")
			}
		}
	} else {
		logrus.WithError(err).Error("error unmarshaling dial message")
	}
}

func (self *dialHandler) sendLinkMessage(linkId string) error {
	linkMsg := &ctrl_pb.Link{Id: linkId}
	body, err := proto.Marshal(linkMsg)
	if err == nil {
		msg := channel2.NewMessage(int32(ctrl_pb.ContentType_LinkType), body)
		if err := self.ctrl.Channel().Send(msg); err != nil {
			return errors.Wrap(err, "error sending link message")
		}
	} else {
		return errors.Wrap(err, "error marshaling link message")
	}
	return nil
}

func (self *dialHandler) sendLinkFault(linkId string) error {
	fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: linkId}
	body, err := proto.Marshal(fault)
	if err == nil {
		msg := channel2.NewMessage(int32(ctrl_pb.ContentType_FaultType), body)
		if err := self.ctrl.Channel().Send(msg); err != nil {
			return errors.Wrap(err, "error sending fault")
		}
	} else {
		return errors.Wrapf(err, "error marshaling fault")
	}
	return nil
}
