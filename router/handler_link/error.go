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
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
)

type errorHandler struct {
	link xlink.Xlink
	ctrl xgress.CtrlChannel
}

func newErrorHandler(link xlink.Xlink, ctrl xgress.CtrlChannel) *errorHandler {
	return &errorHandler{link: link, ctrl: ctrl}
}

func (self *errorHandler) HandleError(err error, ch channel2.Channel) {
	log := pfxlog.ContextLogger(self.link.Id().Token)
	fault := &ctrl_pb.Fault{
		Subject: ctrl_pb.FaultSubject_LinkFault,
		Id:      self.link.Id().Token,
	}
	if body, err := proto.Marshal(fault); err == nil {
		msg := channel2.NewMessage(int32(ctrl_pb.ContentType_FaultType), body)
		if err := self.ctrl.Channel().Send(msg); err == nil {
			log.Infof("transmitted link fault")
		} else {
			log.Errorf("unexpected error sending fault (%v)", err)
		}
	}
	self.link.Close()
}
