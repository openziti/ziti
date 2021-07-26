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

package handler_mgmt

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/events"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
)

type streamCircuitsHandler struct {
	network        *network.Network
	streamHandlers []network.CircuitEventHandler
}

func newStreamCircuitsHandler(network *network.Network) *streamCircuitsHandler {
	return &streamCircuitsHandler{network: network}
}

func (*streamCircuitsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_StreamCircuitsRequestType)
}

func (handler *streamCircuitsHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	request := &mgmt_pb.StreamCircuitsRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}

	circuitsStreamHandler := &CircuitsStreamHandler{ch: ch}
	handler.streamHandlers = append(handler.streamHandlers, circuitsStreamHandler)
	events.AddCircuitEventHandler(circuitsStreamHandler)
}

func (handler *streamCircuitsHandler) HandleClose(ch channel2.Channel) {
	for _, listener := range handler.streamHandlers {
		events.RemoveCircuitEventHandler(listener)
	}
}

type CircuitsStreamHandler struct {
	ch channel2.Channel
}

func (handler *CircuitsStreamHandler) CircuitCreated(circuitId string, clientId string, serviceId string, path *network.Path) {
	event := &mgmt_pb.StreamCircuitsEvent{
		EventType: mgmt_pb.StreamCircuitEventType_CircuitCreated,
		CircuitId: circuitId,
		ClientId:  clientId,
		ServiceId: serviceId,
		Path:      NewPath(path),
	}
	handler.sendEvent(event)
}

func (handler *CircuitsStreamHandler) CircuitDeleted(circuitId string, clientId string) {
	event := &mgmt_pb.StreamCircuitsEvent{
		EventType: mgmt_pb.StreamCircuitEventType_CircuitDeleted,
		CircuitId: circuitId,
		ClientId:  clientId,
	}
	handler.sendEvent(event)
}

func (handler *CircuitsStreamHandler) PathUpdated(circuitId string, path *network.Path) {
	event := &mgmt_pb.StreamCircuitsEvent{
		EventType: mgmt_pb.StreamCircuitEventType_PathUpdated,
		CircuitId: circuitId,
		Path:      NewPath(path),
	}
	handler.sendEvent(event)
}

func (handler *CircuitsStreamHandler) sendEvent(event *mgmt_pb.StreamCircuitsEvent) {
	body, err := proto.Marshal(event)
	if err != nil {
		pfxlog.Logger().Errorf("unexpected error serializing StreamCircuitsEvent (%s)", err)
		return
	}

	responseMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_StreamCircuitsEventType), body)
	if err := handler.ch.Send(responseMsg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending StreamMetricsEvent (%s)", err)
		handler.close()
	}
}

func (handler *CircuitsStreamHandler) close() {
	if err := handler.ch.Close(); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error closing mgmt channel")
	}
	events.RemoveCircuitEventHandler(handler)
}
