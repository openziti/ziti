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
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/event"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/handler_common"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"io"
)

type StreamEventsRequest struct {
	Format        string                `json:"format"`
	Subscriptions []*event.Subscription `json:"subscriptions"`
}

type streamEventsHandler struct {
	network             *network.Network
	eventStreamHandlers []io.Closer
}

func newStreamEventsHandler(network *network.Network) *streamEventsHandler {
	return &streamEventsHandler{network: network}
}

func (*streamEventsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_StreamEventsRequestType)
}

func (handler *streamEventsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	dispatcher := handler.network.GetEventDispatcher()

	request := &StreamEventsRequest{}
	if err := json.Unmarshal(msg.Body, request); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	formatterFactory := dispatcher.GetFormatterFactory(request.Format)
	if formatterFactory == nil {
		handler_common.SendFailure(msg, ch, fmt.Sprintf("invalid format ['%v']", request.Format))
		return
	}

	formatter := formatterFactory.NewFormatter(&EventsStreamHandler{
		ch: ch,
	})

	handler.eventStreamHandlers = append(handler.eventStreamHandlers, formatter)

	if err := handler.network.GetEventDispatcher().ProcessSubscriptions(formatter, request.Subscriptions); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
	} else {
		handler_common.SendSuccess(msg, ch, "success")
	}
}

func (handler *streamEventsHandler) HandleClose(channel.Channel) {
	for _, streamHandler := range handler.eventStreamHandlers {
		handler.network.GetEventDispatcher().RemoveAllSubscriptions(streamHandler)
		if err := streamHandler.Close(); err != nil {
			pfxlog.Logger().WithError(err).Error("error while closing stream event handler")
		}
	}
}

type EventsStreamHandler struct {
	ch channel.Channel
}

func (handler *EventsStreamHandler) AcceptFormattedEvent(eventType string, formattedEvent []byte) {
	msg := channel.NewMessage(int32(mgmt_pb.ContentType_StreamEventsEventType), formattedEvent)
	msg.PutStringHeader(int32(mgmt_pb.Header_EventTypeHeader), eventType)
	if err := handler.ch.Send(msg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending StreamEventsEvent (%s)", err)
		handler.close()
	}
}

func (handler *EventsStreamHandler) close() {
	if err := handler.ch.Close(); err != nil {
		pfxlog.Logger().WithError(err).Errorf("failure while closing handler")
	}
}
