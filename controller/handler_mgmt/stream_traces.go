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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/trace/pb"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/handler_common"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/fabric/trace"
	"google.golang.org/protobuf/proto"
)

type streamTracesHandler struct {
	network        *network.Network
	streamHandlers []trace.EventHandler
}

func newStreamTracesHandler(network *network.Network) *streamTracesHandler {
	return &streamTracesHandler{network: network}
}

func (*streamTracesHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_StreamTracesRequestType)
}

func (handler *streamTracesHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	request := &mgmt_pb.StreamTracesRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	filter := createFilter(request)
	eventHandler := &traceEventsHandler{ch, filter}

	handler.streamHandlers = append(handler.streamHandlers, eventHandler)
	trace.AddTraceEventHandler(eventHandler)
}

func (handler *streamTracesHandler) HandleClose(channel.Channel) {
	for _, streamHandler := range handler.streamHandlers {
		trace.RemoveTraceEventHandler(streamHandler)
	}
}

func createFilter(request *mgmt_pb.StreamTracesRequest) trace.Filter {
	if !request.EnabledFilter {
		return trace.NewAllowAllFilter()
	}
	if request.FilterType == mgmt_pb.TraceFilterType_INCLUDE {
		return trace.NewIncludeFilter(request.ContentTypes)
	}
	return trace.NewExcludeFilter(request.ContentTypes)
}

type traceEventsHandler struct {
	ch     channel.Channel
	filter trace.Filter
}

func (handler *traceEventsHandler) Accept(event *trace_pb.ChannelMessage) {
	if !handler.filter.Accept(event) {
		return
	}
	body, err := proto.Marshal(event)
	if err != nil {
		pfxlog.Logger().Errorf("unexpected error unmarshalling ChannelMessage (%s)", err)
		return
	}

	responseMsg := channel.NewMessage(int32(mgmt_pb.ContentType_StreamTracesEventType), body)
	if err := handler.ch.Send(responseMsg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending ChannelMessage (%s)", err)
		handler.close()
	}
}

func (handler *traceEventsHandler) close() {
	if err := handler.ch.Close(); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error while closing mgmt channel")
	}
}
