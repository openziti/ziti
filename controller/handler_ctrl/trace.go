package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/trace/pb"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"github.com/openziti/fabric/trace"
	"google.golang.org/protobuf/proto"
)

type traceHandler struct {
	dispatcher trace.EventHandler
}

func newTraceHandler(dispatcher trace.EventHandler) *traceHandler {
	return &traceHandler{dispatcher: dispatcher}
}

func (*traceHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_TraceEventType)
}

func (handler *traceHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	event := &trace_pb.ChannelMessage{}
	if err := proto.Unmarshal(msg.Body, event); err == nil {
		go handler.dispatcher.Accept(event)
	} else {
		pfxlog.Logger().Errorf("unexpected error decoding trace message (%s)", err)
	}
}
