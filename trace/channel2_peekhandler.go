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

package trace

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	trace_pb "github.com/openziti/foundation/trace/pb"
	"github.com/openziti/foundation/util/concurrenz"
	"time"
)

var channel2decoders = []channel2.TraceMessageDecoder{&channel2.Decoder{}, ctrl_pb.Channel2Decoder{}, xgress.Channel2Decoder{}, mgmt_pb.Channel2Decoder{}}

type Channel2PeekHandler struct {
	appId      string
	ch         channel2.Channel
	enabled    concurrenz.AtomicBoolean
	controller Controller
	decoders   []channel2.TraceMessageDecoder
	eventSink  EventHandler
}

func (handler *Channel2PeekHandler) EnableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult) {
	handler.ToggleTracing(sourceType, matcher, true, resultChan)
}

func (handler *Channel2PeekHandler) DisableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult) {
	handler.ToggleTracing(sourceType, matcher, false, resultChan)
}

func (handler *Channel2PeekHandler) ToggleTracing(sourceType SourceType, matcher SourceMatcher, enable bool, resultChan chan<- ToggleApplyResult) {
	name := handler.ch.LogicalName()
	matched := sourceType == SourceTypePipe && matcher.Matches(name)
	prevState := handler.IsEnabled()
	nextState := prevState
	if matched {
		handler.enable(enable)
		nextState = enable
	}
	resultChan <- &ToggleApplyResultImpl{matched,
		fmt.Sprintf("Link %v.%v matched? %v. Old trace state: %v, New trace state: %v",
			handler.appId, name, matched, prevState, nextState)}
}

func NewChannel2PeekHandler(appId string, ch channel2.Channel, controller Controller, eventSink EventHandler) *Channel2PeekHandler {
	handler := &Channel2PeekHandler{
		appId:      appId,
		ch:         ch,
		controller: controller,
		decoders:   channel2decoders,
		eventSink:  eventSink,
	}
	controller.AddSource(handler)
	return handler
}

func (handler *Channel2PeekHandler) enable(enabled bool) {
	handler.enabled.Set(true)
}

func (handler *Channel2PeekHandler) IsEnabled() bool {
	return handler.enabled.Get()
}

func (*Channel2PeekHandler) Connect(ch channel2.Channel, remoteAddress string) {
}

func (handler *Channel2PeekHandler) Rx(msg *channel2.Message, ch channel2.Channel) {
	handler.trace(msg, ch, false)
}

func (handler *Channel2PeekHandler) Tx(msg *channel2.Message, ch channel2.Channel) {
	handler.trace(msg, ch, true)
}

func (handler *Channel2PeekHandler) Close(ch channel2.Channel) {
	handler.controller.RemoveSource(handler)
}

func (handler *Channel2PeekHandler) trace(msg *channel2.Message, ch channel2.Channel, rx bool) {
	if !handler.IsEnabled() || msg.ContentType == int32(ctrl_pb.ContentType_TraceEventType) ||
		msg.ContentType == int32(mgmt_pb.ContentType_StreamTracesEventType) {
		return
	}

	var decode []byte
	for _, decoder := range handler.decoders {
		if str, ok := decoder.Decode(msg); ok {
			decode = str
			break
		}
	}

	traceMsg := &trace_pb.ChannelMessage{
		Timestamp:   time.Now().UnixNano(),
		Identity:    handler.appId,
		Channel:     ch.LogicalName(),
		IsRx:        rx,
		ContentType: msg.ContentType,
		Sequence:    msg.Sequence(),
		ReplyFor:    msg.ReplyFor(),
		Length:      int32(len(msg.Body)),
		Decode:      decode,
	}

	// This can result in a message send. Doing a send from inside a peekhandler can cause deadlocks, so it's best avoided
	go handler.eventSink.Accept(traceMsg)
}

func NewChannel2Sink(ch channel2.Channel) EventHandler {
	return &channel2Sink{ch}
}

type channel2Sink struct {
	ch channel2.Channel
}

func (sink *channel2Sink) Accept(event *trace_pb.ChannelMessage) {
	log := pfxlog.Logger()

	bytes, err := proto.Marshal(event)
	if err != nil {
		log.Errorf("Failed to encode metrics message: %v", err)
		return
	}

	chMsg := channel2.NewMessage(int32(ctrl_pb.ContentType_TraceEventType), bytes)

	err = sink.ch.Send(chMsg)
	if err != nil {
		log.Errorf("Failed to send trace message: %v", err)
	} else {
		log.Tracef("Reported trace to fabric controller")
	}
}
