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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/forwarder"
	"github.com/openziti/ziti/router/xgress_router"
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

func (context *inspectRequestContext) inspectXgressDialer(binding string, requested string) {
	factory, _ := xgress_router.GlobalRegistry().Factory(binding)
	if factory == nil {
		context.appendError("no xgress factory configured for edge binding")
		return
	}

	dialer, err := factory.CreateDialer(context.handler.env.GetDialerCfg()[binding])
	if err != nil {
		context.appendError(fmt.Sprintf("could not create %s dialer: (%s)", binding, err.Error()))
		return
	}

	inspectable, ok := dialer.(xgress_router.Inspectable)
	if !ok {
		context.appendError(fmt.Sprintf("%s dialer is not of type Inspectable", binding))
		return
	}

	result := inspectable.Inspect(strings.ToLower(requested), time.Second)
	context.handleJsonResponse(requested, result)
}

func (context *inspectRequestContext) processLocal() {
	for _, requested := range context.request.RequestedValues {
		lc := strings.ToLower(requested)
		if lc == "stackdump" {
			context.appendValue(requested, debugz.GenerateStack())
		} else if lc == "links" {
			result := context.handler.env.GetXlinkRegistry().Inspect(time.Second)
			context.handleJsonResponse(requested, result)
		} else if lc == inspect.SdkTerminatorsKey {
			context.inspectXgressDialer("edge", requested)
		} else if lc == inspect.ErtTerminatorsKey {
			context.inspectXgressDialer("tunnel", requested)
		} else if strings.HasPrefix(lc, "circuit:") {
			circuitId := requested[len("circuit:"):]
			result := context.handler.fwd.InspectCircuit(circuitId, false)
			if result != nil {
				context.handleJsonResponse(requested, result)
			}
		} else if strings.HasPrefix(lc, "circuitandstacks:") {
			circuitId := requested[len("circuitAndStacks:"):]
			result := context.handler.fwd.InspectCircuit(circuitId, true)
			if result != nil {
				context.handleJsonResponse(requested, result)
			}
		} else if lc == "router-circuits" {
			result := context.handler.fwd.InspectCircuits()
			context.handleJsonResponse(requested, result)
		} else if strings.HasPrefix(lc, "metrics") {
			msg := context.handler.fwd.MetricsRegistry().PollWithoutUsageMetrics()
			context.handleJsonResponse(requested, msg)
		} else if lc == "config" {
			js, err := context.handler.env.RenderJsonConfig()
			if err != nil {
				context.appendError(err.Error())
			} else {
				context.appendValue(requested, js)
			}
		} else if lc == "router-data-model" {
			result := context.handler.env.GetRouterDataModel()
			context.handleJsonResponse(requested, result)
		} else if lc == "router-data-model-index" {
			idx, _ := context.handler.env.GetRouterDataModel().CurrentIndex()
			data := map[string]any{
				"timeline": context.handler.env.GetRouterDataModel().GetTimelineId(),
				"index":    idx,
			}
			context.handleJsonResponse(requested, data)
		} else if lc == "router-controllers" {
			result := context.handler.env.GetNetworkControllers().Inspect()
			context.handleJsonResponse(requested, result)
		} else if lc == inspect.RouterIdentityConnectionStatusesKey {
			factory, _ := xgress_router.GlobalRegistry().Factory("edge")
			if factory == nil {
				context.appendError("no xgress factory configured for edge binding")
				continue
			}

			inspectable, ok := factory.(xgress_router.Inspectable)
			if !ok {
				context.appendError("edge factory is not of type Inspectable")
				continue
			}

			result := inspectable.Inspect(lc, time.Second)
			context.handleJsonResponse(requested, result)
		} else if strings.EqualFold(lc, "router-edge-circuits") || strings.EqualFold(lc, "router-sdk-circuits") {
			context.inspectXgListener(requested)
		}
	}
}

func (context *inspectRequestContext) inspectXgListener(val string) {
	for _, l := range context.handler.env.GetXgressListeners() {
		if inspectable, ok := l.(xgress_router.Inspectable); ok {
			if result := inspectable.Inspect(val, time.Second); result != nil {
				context.handleJsonResponse(val, result)
				break
			}
		}
	}
}

func (context *inspectRequestContext) handleJsonResponse(key string, val interface{}) {
	js, err := json.Marshal(val)
	if err != nil {
		context.appendError(errors.Wrapf(err, "failed to marshall %s to json", key).Error())
	} else {
		context.appendValue(key, string(js))
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
