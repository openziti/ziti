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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"github.com/openziti/fabric/router/env"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type dialHandler struct {
	env env.RouterEnv
}

func newDialHandler(env env.RouterEnv) *dialHandler {
	handler := &dialHandler{
		env: env,
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

	log := self.getLogger(dial)

	if len(self.env.GetXlinkDialers()) == 0 {
		log.Errorf("invalid Xlink dialers configuration")
		go self.sendLinkFault(dial, ch)
		return
	}

	self.env.GetXlinkRegistry().DialRequested(ch, dial)
	go self.sendLinkFault(dial, ch)
}

func (self *dialHandler) sendLinkFault(dial *ctrl_pb.Dial, ch channel.Channel) {
	fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: dial.LinkId}
	if err := protobufs.MarshalTyped(fault).Send(ch); err != nil {
		self.getLogger(dial).WithError(err).Error("error sending link fault")
	}
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
