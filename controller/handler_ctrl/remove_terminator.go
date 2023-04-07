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
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/handler_common"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"google.golang.org/protobuf/proto"
)

type removeTerminatorHandler struct {
	baseHandler
}

func newRemoveTerminatorHandler(network *network.Network, router *network.Router) *removeTerminatorHandler {
	return &removeTerminatorHandler{
		baseHandler: baseHandler{
			router:  router,
			network: network,
		},
	}
}

func (self *removeTerminatorHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_RemoveTerminatorRequestType)
}

func (self *removeTerminatorHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	request := &ctrl_pb.RemoveTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		log.WithError(err).Error("failed to unmarshal remove terminator message")
		return
	}

	go self.handleRemoveTerminator(msg, ch, request)
}

func (self *removeTerminatorHandler) handleRemoveTerminator(msg *channel.Message, ch channel.Channel, request *ctrl_pb.RemoveTerminatorRequest) {
	log := pfxlog.ContextLogger(ch.Label())

	terminator, err := self.network.Terminators.Read(request.TerminatorId)
	if err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	if err := self.network.Terminators.Delete(request.TerminatorId, self.newChangeContext(ch, "fabric.remove.terminator")); err == nil {
		log.
			WithField("routerId", ch.Id()).
			WithField("serviceId", terminator.Service).
			WithField("terminator", request.TerminatorId).
			Info("removed terminator")
		handler_common.SendSuccess(msg, ch, "")
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}
