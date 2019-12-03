/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-fabric/fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/util/debugz"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"strings"
)

type inspectHandler struct {
	id *identity.TokenId
}

func newInspectHandler(id *identity.TokenId) *inspectHandler {
	return &inspectHandler{id: id}
}

func (*inspectHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_InspectRequestType)
}

func (handler *inspectHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	context := &inspectRequestContext{
		handler:  handler,
		msg:      msg,
		ch:       ch,
		request:  &ctrl_pb.InspectRequest{},
		response: &ctrl_pb.InspectResponse{Success: true},
	}

	var err error
	if err = proto.Unmarshal(msg.Body, context.request); err != nil {
		context.appendError(err.Error())
	}

	if !context.response.Success {
		context.sendResponse()
		return
	}

	context.processLocal()
	context.sendResponse()
}

type inspectRequestContext struct {
	handler  *inspectHandler
	msg      *channel2.Message
	ch       channel2.Channel
	request  *ctrl_pb.InspectRequest
	response *ctrl_pb.InspectResponse
}

func (context *inspectRequestContext) processLocal() {
	for _, requested := range context.request.RequestedValues {
		if strings.ToLower(requested) == "stackdump" {
			context.appendValue(context.handler.id, requested, debugz.GenerateStack())
		}
	}
}

func (context *inspectRequestContext) sendResponse() {
	body, err := proto.Marshal(context.response)
	if err != nil {
		pfxlog.Logger().Errorf("unexpected error serializing InspectResponse (%s)", err)
		return
	}

	responseMsg := channel2.NewMessage(int32(ctrl_pb.ContentType_InspectResponseType), body)
	responseMsg.ReplyTo(context.msg)
	if err := context.ch.Send(responseMsg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending InspectResponse (%s)", err)
	}
}

func (context *inspectRequestContext) appendValue(appId *identity.TokenId, name string, value string) {
	context.response.Values = append(context.response.Values, &ctrl_pb.InspectResponse_InspectValue{
		Name:  name,
		Value: value,
	})
}

func (context *inspectRequestContext) appendError(err string) {
	context.response.Success = false
	context.response.Errors = append(context.response.Errors, err)
}
