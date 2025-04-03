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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/common/pb/mgmt_pb"
	"github.com/openziti/ziti/controller/network"
	"google.golang.org/protobuf/proto"
	"time"
)

type validateRouterErtTerminatorsHandler struct {
	network *network.Network
}

func newValidateRouterErtTerminatorsHandler(network *network.Network) *validateRouterErtTerminatorsHandler {
	return &validateRouterErtTerminatorsHandler{network: network}
}

func (*validateRouterErtTerminatorsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ValidateRouterErtTerminatorsRequestType)
}

func (handler *validateRouterErtTerminatorsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	request := &mgmt_pb.ValidateRouterErtTerminatorsRequest{}

	var err error

	var count int64
	var evalF func()
	if err = proto.Unmarshal(msg.Body, request); err == nil {
		count, evalF, err = handler.network.ValidateRouterErtTerminators(request.Filter, func(detail *mgmt_pb.RouterErtTerminatorsDetails) {
			if !ch.IsClosed() {
				if sendErr := protobufs.MarshalTyped(detail).WithTimeout(15 * time.Second).SendAndWaitForWire(ch); sendErr != nil {
					log.WithError(sendErr).Error("send of ert terminators detail failed, closing channel")
					if closeErr := ch.Close(); closeErr != nil {
						log.WithError(closeErr).Error("failed to close channel")
					}
				}
			} else {
				log.Info("channel closed, unable to send ert terminators detail")
			}
		})
	}

	response := &mgmt_pb.ValidateRouterErtTerminatorsResponse{
		Success:     err == nil,
		RouterCount: uint64(count),
	}
	if err != nil {
		response.Message = fmt.Sprintf("%v: failed to unmarshall request: %v", handler.network.GetAppId(), err)
	}

	body, err := proto.Marshal(response)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error serializing ValidateRouterErtTerminatorsResponse")
		return
	}

	responseMsg := channel.NewMessage(int32(mgmt_pb.ContentType_ValidateRouterErtTerminatorsResponseType), body)
	responseMsg.ReplyTo(msg)
	if err = ch.Send(responseMsg); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error sending ValidateRouterErtTerminatorsResponse")
	}

	if evalF != nil {
		evalF()
	}
}
