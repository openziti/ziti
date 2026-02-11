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

package handler_mgmt

import (
	"fmt"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/network"
	"google.golang.org/protobuf/proto"
)

type validateControllerDialersHandler struct {
	network *network.Network
}

func newValidateControllerDialersHandler(network *network.Network) channel.TypedReceiveHandler {
	return &validateControllerDialersHandler{network: network}
}

func (*validateControllerDialersHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ValidateControllerDialersRequestType)
}

func (handler *validateControllerDialersHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	request := &mgmt_pb.ValidateControllerDialersRequest{}

	var err error
	if err = proto.Unmarshal(msg.Body, request); err != nil {
		log.WithError(err).Error("failed to unmarshal request")
		return
	}

	validator := handler.network.GetCtrlDialerValidator()

	var details []*mgmt_pb.ControllerDialerDetails
	if validator != nil {
		details, err = validator()
	} else {
		err = fmt.Errorf("controller dialer is not enabled")
	}

	count := int64(len(details))

	response := &mgmt_pb.ValidateControllerDialersResponse{
		Success:        err == nil,
		ComponentCount: uint64(count),
	}
	if err != nil {
		response.Message = fmt.Sprintf("%v: %v", handler.network.GetAppId(), err)
	}

	body, err := proto.Marshal(response)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error serializing ValidateControllerDialersResponse")
		return
	}

	responseMsg := channel.NewMessage(int32(mgmt_pb.ContentType_ValidateControllerDialersResponseType), body)
	responseMsg.ReplyTo(msg)
	if err = ch.Send(responseMsg); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error sending ValidateControllerDialersResponse")
		return
	}

	for _, detail := range details {
		if !ch.IsClosed() {
			if sendErr := protobufs.MarshalTyped(detail).WithTimeout(15 * time.Second).SendAndWaitForWire(ch); sendErr != nil {
				log.WithError(sendErr).Error("send of controller dialer detail failed, closing channel")
				if closeErr := ch.Close(); closeErr != nil {
					log.WithError(closeErr).Error("failed to close channel")
				}
				return
			}
		} else {
			log.Info("channel closed, unable to send controller dialer detail")
			return
		}
	}
}
