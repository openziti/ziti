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

package trace

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/channel/trace/pb"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/v2/concurrenz"
	"google.golang.org/protobuf/proto"
	"time"
)

var decoders = []channel.TraceMessageDecoder{channel.Decoder{}, ctrl_pb.Decoder{}, xgress.Decoder{}, mgmt_pb.Decoder{}}

type ChannelPeekHandler struct {
	appId      string
	ch         channel.Channel
	enabled    concurrenz.AtomicBoolean
	controller Controller
	decoders   []channel.TraceMessageDecoder
	eventSinks concurrenz.CopyOnWriteSlice[EventHandler]
}

func (self *ChannelPeekHandler) EnableTracing(sourceType SourceType, matcher SourceMatcher, handler EventHandler, resultChan chan<- ToggleApplyResult) {
	self.ToggleTracing(sourceType, matcher, true, handler, resultChan)
}

func (self *ChannelPeekHandler) DisableTracing(sourceType SourceType, matcher SourceMatcher, handler EventHandler, resultChan chan<- ToggleApplyResult) {
	self.ToggleTracing(sourceType, matcher, false, handler, resultChan)
}

func (self *ChannelPeekHandler) ToggleTracing(sourceType SourceType, matcher SourceMatcher, enable bool, handler EventHandler, resultChan chan<- ToggleApplyResult) {
	name := self.ch.LogicalName()
	matched := sourceType == SourceTypePipe && matcher.Matches(name)
	prevState := self.IsEnabled()
	nextState := prevState

	if matched {
		nextState = enable
		if enable {
			self.enabled.Set(true)
			self.eventSinks.Append(handler)
		} else {
			self.eventSinks.Delete(handler)
			if len(self.eventSinks.Value()) == 0 {
				self.enabled.Set(false)
			}
		}
	}

	resultChan <- &ToggleApplyResultImpl{matched,
		fmt.Sprintf("Link %v.%v matched? %v. Old trace state: %v, New trace state: %v",
			self.appId, name, matched, prevState, nextState)}
}

func NewChannelPeekHandler(appId string, ch channel.Channel, controller Controller) *ChannelPeekHandler {
	handler := &ChannelPeekHandler{
		appId:      appId,
		ch:         ch,
		controller: controller,
		decoders:   decoders,
	}
	controller.AddSource(handler)
	return handler
}

func (self *ChannelPeekHandler) IsEnabled() bool {
	return self.enabled.Get()
}

func (*ChannelPeekHandler) Connect(channel.Channel, string) {
}

func (self *ChannelPeekHandler) Rx(msg *channel.Message, ch channel.Channel) {
	self.trace(msg, ch, false)
}

func (self *ChannelPeekHandler) Tx(msg *channel.Message, ch channel.Channel) {
	self.trace(msg, ch, true)
}

func (self *ChannelPeekHandler) Close(channel.Channel) {
	self.controller.RemoveSource(self)
}

func (self *ChannelPeekHandler) trace(msg *channel.Message, ch channel.Channel, rx bool) {
	if !self.IsEnabled() || msg.ContentType == int32(ctrl_pb.ContentType_TraceEventType) ||
		msg.ContentType == int32(mgmt_pb.ContentType_StreamTracesEventType) {
		return
	}

	var decode []byte
	for _, decoder := range self.decoders {
		if str, ok := decoder.Decode(msg); ok {
			decode = str
			break
		}
	}

	traceMsg := &trace_pb.ChannelMessage{
		Timestamp:   time.Now().UnixNano(),
		Identity:    self.appId,
		Channel:     ch.LogicalName(),
		IsRx:        rx,
		ContentType: msg.ContentType,
		Sequence:    msg.Sequence(),
		ReplyFor:    msg.ReplyFor(),
		Length:      int32(len(msg.Body)),
		Decode:      decode,
	}

	// This can result in a message send. Doing a send from inside a peekhandler can cause deadlocks, so it's best avoided
	for _, eventSink := range self.eventSinks.Value() {
		go eventSink.Accept(traceMsg)
	}
}

func NewChannelSink(ch channel.Channel) EventHandler {
	return &channelSink{ch}
}

type channelSink struct {
	ch channel.Channel
}

func (sink *channelSink) Accept(event *trace_pb.ChannelMessage) {
	log := pfxlog.Logger()

	bytes, err := proto.Marshal(event)
	if err != nil {
		log.Errorf("Failed to encode metrics message: %v", err)
		return
	}

	chMsg := channel.NewMessage(int32(ctrl_pb.ContentType_TraceEventType), bytes)

	err = sink.ch.Send(chMsg)
	if err != nil {
		log.Errorf("Failed to send trace message: %v", err)
	} else {
		log.Tracef("Reported trace to fabric controller")
	}
}
