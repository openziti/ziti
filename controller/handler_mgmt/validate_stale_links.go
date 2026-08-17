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
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/network"
	"google.golang.org/protobuf/proto"
)

// validateStaleLinksHandler is the mgmt-plane entry point for the
// stale-link GC tool. It kicks off Network.ValidateStaleLinks and
// streams per-link StaleLinkResult messages back to the client.
type validateStaleLinksHandler struct {
	network *network.Network
}

func newValidateStaleLinksHandler(network *network.Network) *validateStaleLinksHandler {
	return &validateStaleLinksHandler{network: network}
}

func (*validateStaleLinksHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ValidateStaleLinksRequestType)
}

func (handler *validateStaleLinksHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	request := &mgmt_pb.ValidateStaleLinksRequest{}

	var err error
	var count int64
	var evalF func()
	if err = proto.Unmarshal(msg.Body, request); err == nil {
		count, evalF, err = handler.network.ValidateStaleLinks(request.Filter, request.Mode, request.Gc, func(result *mgmt_pb.StaleLinkResult) {
			if ch.IsClosed() {
				log.Info("channel closed, unable to send stale-link result")
				return
			}
			if sendErr := protobufs.MarshalTyped(result).WithTimeout(15 * time.Second).SendAndWaitForWire(ch); sendErr != nil {
				log.WithError(sendErr).Error("send of stale-link result failed, closing channel")
				if closeErr := ch.Close(); closeErr != nil {
					log.WithError(closeErr).Error("failed to close channel")
				}
			}
		})
	}

	response := &mgmt_pb.ValidateStaleLinksResponse{
		Success:           err == nil,
		ExpectedLinkCount: uint64(count),
	}
	if err != nil {
		response.Message = fmt.Sprintf("%v: %v", handler.network.GetAppId(), err)
	}

	body, err := proto.Marshal(response)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error serializing ValidateStaleLinksResponse")
		return
	}

	responseMsg := channel.NewMessage(int32(mgmt_pb.ContentType_ValidateStaleLinksResponseType), body)
	responseMsg.ReplyTo(msg)
	if err = ch.Send(responseMsg); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error sending ValidateStaleLinksResponse")
	}

	if evalF != nil {
		evalF()
	}
}
