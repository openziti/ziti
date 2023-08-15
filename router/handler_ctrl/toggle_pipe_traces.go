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
	trace_pb "github.com/openziti/channel/v2/trace/pb"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/identity"
	"google.golang.org/protobuf/proto"
)

func newTraceHandler(appId *identity.TokenId, controller trace.Controller, ctrlCh channel.Channel) *traceHandler {
	return &traceHandler{
		appId:        appId,
		controller:   controller,
		enabled:      false,
		eventHandler: trace.NewChannelSink(ctrlCh),
	}
}

type traceHandler struct {
	appId        *identity.TokenId
	controller   trace.Controller
	enabled      bool
	eventHandler trace.EventHandler
}

func (*traceHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_TogglePipeTracesRequestType)
}

func (handler *traceHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	request := &trace_pb.TogglePipeTracesRequest{}

	if err := proto.Unmarshal(msg.Body, request); err != nil {
		handler.sendFailure(msg, ch, err.Error())
		return
	}

	matchers, result := trace.NewPipeToggleMatchers(request)

	if result.Success {
		resultChan := make(chan trace.ToggleApplyResult)

		if matchers.AppMatcher.Matches(handler.appId.Token) {
			if request.Enable {
				handler.controller.EnableTracing(trace.SourceTypePipe, matchers.PipeMatcher, handler.eventHandler, resultChan)
			} else {
				handler.controller.DisableTracing(trace.SourceTypePipe, matchers.PipeMatcher, handler.eventHandler, resultChan)
			}
		}

		verbosity := trace.GetVerbosity(request.Verbosity)
		for applyResult := range resultChan {
			applyResult.Append(result, verbosity)
		}
	}

	if result.Success {
		handler.sendSuccess(msg, ch, result.Message.String())
	} else {
		handler.sendFailure(msg, ch, result.Message.String())
	}
}

func (handler *traceHandler) sendSuccess(request *channel.Message, ch channel.Channel, message string) {
	handler.sendResult(request, ch, message, true)
}

func (handler *traceHandler) sendFailure(request *channel.Message, ch channel.Channel, message string) {
	handler.sendResult(request, ch, message, false)
}

func (handler *traceHandler) sendResult(request *channel.Message, ch channel.Channel, message string, success bool) {
	log := pfxlog.ContextLogger(ch.Label()).WithField("operation", "togglePipeTraces")
	if !success {
		log.Errorf("ctrl error (%s)", message)
	}

	response := channel.NewResult(success, message)
	response.ReplyTo(request)
	if err := ch.Send(response); err != nil {
		log.Error("failed to send response to toggle pipe traces")
	} else {
		log.Debug("sent response to toggle pipe traces")
	}
}
