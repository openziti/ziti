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

package handler_mgmt

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-fabric/fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-fabric/fabric/trace"
	"github.com/netfoundry/ziti-foundation/trace/pb"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
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

func (handler *streamTracesHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	request := &mgmt_pb.StreamTracesRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		sendFailure(msg, ch, err.Error())
		return
	}

	eventRouter := handler.network.GetTraceEventController()

	filter := createFilter(request)
	eventHandler := &traceEventsHandler{ch, eventRouter, filter}

	handler.streamHandlers = append(handler.streamHandlers, eventHandler)
	eventRouter.AddHandler(eventHandler)
}

func (handler *streamTracesHandler) HandleClose(ch channel2.Channel) {
	for _, streamHandler := range handler.streamHandlers {
		handler.network.GetTraceEventController().RemoveHandler(streamHandler)
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
	ch     channel2.Channel
	router trace.EventController
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

	responseMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_StreamTracesEventType), body)
	if err := handler.ch.Send(responseMsg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending ChannelMessage (%s)", err)
		handler.close()
	}
}

func (handler *traceEventsHandler) close() {
	if err := handler.ch.Close(); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error while closing mgmt channel")
	}
	handler.router.RemoveHandler(handler)
}
