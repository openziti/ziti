/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-fabric/trace"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	trace_pb "github.com/netfoundry/ziti-foundation/trace/pb"
)

func newTraceHandler(appId *identity.TokenId, controller trace.Controller) *traceHandler {
	return &traceHandler{
		appId:      appId,
		controller: controller,
		enabled:    false,
	}
}

type traceHandler struct {
	appId      *identity.TokenId
	controller trace.Controller
	enabled    bool
}

func (*traceHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_TogglePipeTracesRequestType)
}

func (handler *traceHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	request := &trace_pb.TogglePipeTracesRequest{}

	if err := proto.Unmarshal(msg.Body, request); err != nil {
		sendFailure(msg, ch, err.Error())
		return
	}

	matchers, result := trace.NewPipeToggleMatchers(request)

	if result.Success {
		resultChan := make(chan trace.ToggleApplyResult)

		if matchers.AppMatcher.Matches(handler.appId.Token) {
			if request.Enable {
				handler.controller.EnableTracing(trace.SourceTypePipe, matchers.PipeMatcher, resultChan)
			} else {
				handler.controller.DisableTracing(trace.SourceTypePipe, matchers.PipeMatcher, resultChan)
			}
		}

		verbosity := trace.GetVerbosity(request.Verbosity)
		for applyResult := range resultChan {
			applyResult.Append(result, verbosity)
		}
	}

	if result.Success {
		sendSuccess(msg, ch, result.Message.String())
	} else {
		sendFailure(msg, ch, result.Message.String())
	}
}

func sendSuccess(request *channel2.Message, ch channel2.Channel, message string) {
	sendResult(request, ch, message, true)
}

func sendFailure(request *channel2.Message, ch channel2.Channel, message string) {
	sendResult(request, ch, message, false)
}

func sendResult(request *channel2.Message, ch channel2.Channel, message string, success bool) {
	log := pfxlog.ContextLogger(ch.Label())
	if !success {
		log.Errorf("ctrl error (%s)", message)
	}

	response := channel2.NewResult(success, message)
	response.ReplyTo(request)
	ch.Send(response)
	log.Debug("success")
}