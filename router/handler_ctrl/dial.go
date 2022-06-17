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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/env"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/goroutines"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type dialHandler struct {
	id       *identity.TokenId
	ctrl     xgress.CtrlChannel
	dialers  []xlink.Dialer
	registry xlink.Registry
	pool     goroutines.Pool
}

func newDialHandler(env env.RouterEnv, pool goroutines.Pool) *dialHandler {
	handler := &dialHandler{
		id:       env.GetRouterId(),
		ctrl:     env,
		dialers:  env.GetXlinkDialer(),
		pool:     pool,
		registry: env.GetXlinkRegistry(),
	}

	return handler
}

func (self *dialHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_DialType)
}

func (self *dialHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	dial := &ctrl_pb.Dial{}
	if err := proto.Unmarshal(msg.Body, dial); err != nil {
		logrus.WithError(err).Error("error unmarshalling dial message")
		return
	}

	err := self.pool.QueueOrError(func() {
		self.handle(dial, ch)
	})

	if err != nil {
		self.getLogger(dial).WithError(err).Error("error queuing link dial to pool")
	}
}

func (self *dialHandler) handle(dial *ctrl_pb.Dial, _ channel.Channel) {
	log := self.getLogger(dial)

	if len(self.dialers) != 1 {
		log.Errorf("invalid Xlink dialers configuration")
		if err := self.sendLinkFault(dial.LinkId); err != nil {
			log.WithError(err).Error("error sending link fault")
		}
		return
	}

	link, lockAcquired := self.registry.GetDialLock(dial)
	if link != nil && link.Id().Token != dial.LinkId {
		log.WithField("existingLinkId", link.Id().Token).Info("existing link found")
		if err := self.sendLinkFault(dial.LinkId); err != nil {
			log.WithError(err).Error("error sending link fault")
		}
		return
	}

	if lockAcquired {
		log.Info("dialing link")
		if link, err := self.dialers[0].Dial(dial); err == nil {
			if existingLink, success := self.registry.DialSucceeded(link); success {
				log.Info("link registered")
				if err := self.sendLinkMessage(dial.LinkId); err != nil {
					log.WithError(err).Error("error sending link message ")
				}
			} else if existingLink != nil {
				log.WithField("existingLinkId", existingLink.Id().Token).Info("existing link found, new link closed")
			}
		} else {
			log.WithError(err).Error("link dialing failed")
			self.registry.DialFailed(dial)
			if err := self.sendLinkFault(dial.LinkId); err != nil {
				log.WithError(err).Error("error sending fault")
			}
		}
	} else {
		log.Info("unable to dial, dial already in progress")
	}
}

func (self *dialHandler) sendLinkMessage(linkId string) error {
	linkMsg := &ctrl_pb.LinkConnected{Id: linkId}
	if err := protobufs.MarshalTyped(linkMsg).Send(self.ctrl.Channel()); err != nil {
		return errors.Wrap(err, "error sending link message")
	}
	return nil
}

func (self *dialHandler) sendLinkFault(linkId string) error {
	fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: linkId}
	if err := protobufs.MarshalTyped(fault).Send(self.ctrl.Channel()); err != nil {
		return errors.Wrap(err, "error sending fault")
	}
	return nil
}

func (self *dialHandler) getLogger(dial *ctrl_pb.Dial) *logrus.Entry {
	return pfxlog.ChannelLogger("link", "linkDialer").
		WithFields(logrus.Fields{
			"linkId":        dial.LinkId,
			"routerId":      dial.RouterId,
			"address":       dial.Address,
			"linkProtocol":  dial.LinkProtocol,
			"routerVersion": dial.RouterVersion,
		})
}
