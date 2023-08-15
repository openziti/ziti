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
	"encoding/json"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"github.com/openziti/fabric/router/env"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"strings"
	"time"
)

type inspectHandler struct {
	env env.RouterEnv
	fwd *forwarder.Forwarder
}

func newInspectHandler(env env.RouterEnv, fwd *forwarder.Forwarder) *inspectHandler {
	return &inspectHandler{
		env: env,
		fwd: fwd,
	}
}

func (*inspectHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_InspectRequestType)
}

func (handler *inspectHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
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
	msg      *channel.Message
	ch       channel.Channel
	request  *ctrl_pb.InspectRequest
	response *ctrl_pb.InspectResponse
}

func (context *inspectRequestContext) processLocal() {
	for _, requested := range context.request.RequestedValues {
		lc := strings.ToLower(requested)
		if lc == "stackdump" {
			context.appendValue(requested, debugz.GenerateStack())
		} else if lc == "links" {
			result := context.handler.env.GetXlinkRegistry().Inspect(time.Second)
			js, err := json.Marshal(result)
			if err != nil {
				context.appendError(errors.Wrap(err, "failed to marshal links to json").Error())
			} else {
				context.appendValue(requested, string(js))
			}
		} else if strings.HasPrefix(lc, "circuit:") {
			circuitId := requested[len("circuit:"):]
			result := context.handler.fwd.InspectCircuit(circuitId, false)
			if result != nil {
				js, err := json.Marshal(result)
				if err != nil {
					context.appendError(errors.Wrap(err, "failed to marshal circuit report to json").Error())
				} else {
					context.appendValue(requested, string(js))
				}
			}
		} else if strings.HasPrefix(lc, "circuitandstacks:") {
			circuitId := requested[len("circuitAndStacks:"):]
			result := context.handler.fwd.InspectCircuit(circuitId, true)
			if result != nil {
				js, err := json.Marshal(result)
				if err != nil {
					context.appendError(errors.Wrap(err, "failed to marshal circuit report to json").Error())
				} else {
					context.appendValue(requested, string(js))
				}
			}
		} else if strings.HasPrefix(lc, "metrics") {
			msg := context.handler.fwd.MetricsRegistry().PollWithoutUsageMetrics()
			js, err := json.Marshal(msg)
			if err != nil {
				context.appendError(errors.Wrap(err, "failed to marshal metrics to json").Error())
			} else {
				context.appendValue(requested, string(js))
			}
		} else if lc == "config" {
			js, err := context.handler.env.RenderJsonConfig()
			if err != nil {
				context.appendError(err.Error())
			} else {
				context.appendValue(requested, js)
			}
		}
	}
}

func (context *inspectRequestContext) sendResponse() {
	body, err := proto.Marshal(context.response)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error serializing InspectResponse")
		return
	}

	responseMsg := channel.NewMessage(int32(ctrl_pb.ContentType_InspectResponseType), body)
	responseMsg.ReplyTo(context.msg)
	if err := context.ch.Send(responseMsg); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error sending InspectResponse")
	}
}

func (context *inspectRequestContext) appendValue(name string, value string) {
	context.response.Values = append(context.response.Values, &ctrl_pb.InspectResponse_InspectValue{
		Name:  name,
		Value: value,
	})
}

func (context *inspectRequestContext) appendError(err string) {
	context.response.Success = false
	context.response.Errors = append(context.response.Errors, err)
}
