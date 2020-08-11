package handler_ctrl

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/trace"
	"github.com/openziti/foundation/channel2"
	trace_pb "github.com/openziti/foundation/trace/pb"
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

func (handler *traceHandler) HandleReceive(msg *channel2.Message, _ channel2.Channel) {
	event := &trace_pb.ChannelMessage{}
	if err := proto.Unmarshal(msg.Body, event); err == nil {
		handler.dispatcher.Accept(event)
	} else {
		pfxlog.Logger().Errorf("unexpected error decoding trace message (%s)", err)
	}
}
