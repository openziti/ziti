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

package handler_mgmt

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/mgmt_pb"
)

type inspectHandler struct {
	network *network.Network
}

func newInspectHandler(network *network.Network) *inspectHandler {
	return &inspectHandler{network: network}
}

func (*inspectHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_InspectRequestType)
}

func (handler *inspectHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	go func() {
		response := &mgmt_pb.InspectResponse{}
		request := &mgmt_pb.InspectRequest{}
		if err := proto.Unmarshal(msg.Body, request); err != nil {
			response.Success = false
			response.Errors = append(response.Errors, fmt.Sprintf("%v: %v", handler.network.GetAppId(), err))
		} else {
			result := handler.network.Controllers.Inspections.Inspect(request.AppRegex, request.RequestedValues)
			response.Success = result.Success
			response.Errors = result.Errors
			for _, val := range result.Results {
				response.Values = append(response.Values, &mgmt_pb.InspectResponse_InspectValue{
					AppId: val.AppId,
					Name:  val.Name,
					Value: val.Value,
				})
			}
		}

		body, err := proto.Marshal(response)
		if err != nil {
			pfxlog.Logger().Errorf("unexpected error serializing InspectResponse (%s)", err)
			return
		}

		responseMsg := channel.NewMessage(int32(mgmt_pb.ContentType_InspectResponseType), body)
		responseMsg.ReplyTo(msg)
		if err := ch.Send(responseMsg); err != nil {
			pfxlog.Logger().Errorf("unexpected error sending InspectResponse (%s)", err)
		}
	}()
}
