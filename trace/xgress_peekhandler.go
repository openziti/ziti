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
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	trace_pb "github.com/openziti/foundation/trace/pb"
	"sync/atomic"
	"time"
)

type XgressPeekHandler struct {
	appId      *identity.TokenId
	enabled    int32
	controller Controller
	decoders   []channel2.TraceMessageDecoder
	eventSink  EventHandler
}

func (handler *XgressPeekHandler) EnableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult) {
	handler.ToggleTracing(sourceType, matcher, true, resultChan)
}

func (handler *XgressPeekHandler) DisableTracing(sourceType SourceType, matcher SourceMatcher, resultChan chan<- ToggleApplyResult) {
	handler.ToggleTracing(sourceType, matcher, false, resultChan)
}

func (handler *XgressPeekHandler) ToggleTracing(sourceType SourceType, matcher SourceMatcher, enable bool, resultChan chan<- ToggleApplyResult) {
	name := "xgress"
	matched := sourceType == SourceTypePipe && matcher.Matches(name)
	prevState := handler.IsEnabled()
	nextState := prevState
	if matched {
		handler.enable(enable)
		nextState = enable
	}
	resultChan <- &ToggleApplyResultImpl{matched,
		fmt.Sprintf("Link %v.%v matched? %v. Old trace state: %v, New trace state: %v",
			handler.appId.Token, name, matched, prevState, nextState)}
}

func (handler *XgressPeekHandler) Rx(x *xgress.Xgress, payload *xgress.Payload) {
	handler.trace(x, payload, true)
}

func (handler *XgressPeekHandler) Tx(x *xgress.Xgress, payload *xgress.Payload) {
	handler.trace(x, payload, false)
}

func (handler *XgressPeekHandler) Close(x *xgress.Xgress) {
	panic("implement me")
}

func NewXgressPeekHandler(appId *identity.TokenId, controller Controller, eventSink EventHandler) *XgressPeekHandler {

	handler := &XgressPeekHandler{
		appId:      appId,
		enabled:    0,
		controller: controller,
		decoders:   decoders,
		eventSink:  eventSink,
	}
	controller.AddSource(handler)
	return handler
}

func (handler *XgressPeekHandler) enable(enabled bool) {
	atomic.StoreInt32(&handler.enabled, btoi(enabled))
}

func (handler *XgressPeekHandler) IsEnabled() bool {
	return atomic.LoadInt32(&handler.enabled) == 1
}

func (handler *XgressPeekHandler) trace(x *xgress.Xgress, payload *xgress.Payload, rx bool) {

	decode, _ := xgress.DecodePayload(payload)

	traceMsg := &trace_pb.ChannelMessage{
		Timestamp:   time.Now().UnixNano(),
		Identity:    handler.appId.Token,
		Channel:     x.Label(),
		IsRx:        rx,
		ContentType: xgress.ContentTypePayloadType,
		Sequence:    -1,
		ReplyFor:    -1,
		Length:      int32(len(payload.Data)),
		Decode:      decode,
	}

	// This can result in a message send. Doing a send from inside a peekhandler can cause deadlocks, so it's best avoided
	handler.eventSink.Accept(traceMsg)
}
