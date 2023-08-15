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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/common/handler_common"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"google.golang.org/protobuf/proto"
	"math"
)

type createTerminatorHandler struct {
	baseHandler
}

func newCreateTerminatorHandler(network *network.Network, router *network.Router) *createTerminatorHandler {
	return &createTerminatorHandler{
		baseHandler: baseHandler{
			network: network,
			router:  router,
		},
	}
}

func (self *createTerminatorHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_CreateTerminatorRequestType)
}

func (self *createTerminatorHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	request := &ctrl_pb.CreateTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		log.WithError(err).Error("failed to unmarshal create terminator message")
		return
	}
	go self.handleCreateTerminator(msg, ch, request)
}

func (self *createTerminatorHandler) handleCreateTerminator(msg *channel.Message, ch channel.Channel, request *ctrl_pb.CreateTerminatorRequest) {
	if request.Cost > math.MaxUint16 {
		handler_common.SendFailure(msg, ch, fmt.Sprintf("invalid cost %v. cost must be between 0 and %v inclusive", request.Cost, math.MaxUint16))
		return
	}

	terminator := &network.Terminator{
		Service:        request.ServiceId,
		Router:         self.router.Id,
		Binding:        request.Binding,
		Address:        request.Address,
		InstanceId:     request.InstanceId,
		InstanceSecret: request.InstanceSecret,
		PeerData:       request.PeerData,
		Precedence:     request.GetXtPrecedence(),
		Cost:           uint16(request.Cost),
	}

	if err := self.network.Terminators.Create(terminator, self.newChangeContext(ch, "fabric.create.terminator")); err == nil {
		pfxlog.Logger().Infof("created terminator [t/%s]", terminator.Id)
		handler_common.SendSuccess(msg, ch, terminator.Id)
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}
