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
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type closeHandler struct {
	link      *forwarder.Link
	ctrl      xgress.CtrlChannel
	forwarder *forwarder.Forwarder
}

func newCloseHandler(link *forwarder.Link, ctrl xgress.CtrlChannel, forwarder *forwarder.Forwarder) *closeHandler {
	return &closeHandler{link: link, ctrl: ctrl, forwarder: forwarder}
}

func (h *closeHandler) HandleClose(ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	log.Info("link closed")

	fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: h.link.Id.Token}
	if body, err := proto.Marshal(fault); err == nil {
		msg := channel2.NewMessage(int32(ctrl_pb.ContentType_FaultType), body)
		if err := h.ctrl.Channel().Send(msg); err == nil {
			log.Error("transmitted link fault")
		} else {
			log.Errorf("unexpected error transmitting link fault (%s)", err)
		}
	} else {
		log.Errorf("unexpected error (%s)", err)
	}

	h.forwarder.UnregisterLink(h.link)
}
