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
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/event"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"google.golang.org/protobuf/proto"
)

type streamCircuitsHandler struct {
	network        *network.Network
	streamHandlers []event.CircuitEventHandler
}

func newStreamCircuitsHandler(network *network.Network) *streamCircuitsHandler {
	return &streamCircuitsHandler{network: network}
}

func (*streamCircuitsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_StreamCircuitsRequestType)
}

func (handler *streamCircuitsHandler) HandleReceive(_ *channel.Message, ch channel.Channel) {
	circuitsStreamHandler := &CircuitsStreamHandler{ch: ch}
	handler.streamHandlers = append(handler.streamHandlers, circuitsStreamHandler)
	handler.network.GetEventDispatcher().AddCircuitEventHandler(circuitsStreamHandler)
}

func (handler *streamCircuitsHandler) HandleClose(channel.Channel) {
	for _, listener := range handler.streamHandlers {
		handler.network.GetEventDispatcher().RemoveCircuitEventHandler(listener)
	}
}

type CircuitsStreamHandler struct {
	ch channel.Channel
}

func (handler *CircuitsStreamHandler) AcceptCircuitEvent(e *event.CircuitEvent) {
	eventType := mgmt_pb.StreamCircuitEventType_CircuitCreated
	if e.EventType == event.CircuitUpdated {
		eventType = mgmt_pb.StreamCircuitEventType_PathUpdated
	} else if e.EventType == event.CircuitDeleted {
		eventType = mgmt_pb.StreamCircuitEventType_CircuitDeleted
	} else if e.EventType == event.CircuitFailed {
		eventType = mgmt_pb.StreamCircuitEventType_CircuitFailed
	}

	var cts *int64
	if e.CreationTimespan != nil {
		ctsv := int64(*e.CreationTimespan)
		cts = &ctsv
	}

	streamEvent := &mgmt_pb.StreamCircuitsEvent{
		EventType:        eventType,
		CircuitId:        e.CircuitId,
		ClientId:         e.ClientId,
		ServiceId:        e.ServiceId,
		TerminatorId:     e.TerminatorId,
		CreationTimespan: cts,
		Path:             NewPath(&e.Path),
	}
	handler.sendEvent(streamEvent)
}

func (handler *CircuitsStreamHandler) sendEvent(event *mgmt_pb.StreamCircuitsEvent) {
	body, err := proto.Marshal(event)
	if err != nil {
		pfxlog.Logger().Errorf("unexpected error serializing StreamCircuitsEvent (%s)", err)
		return
	}

	responseMsg := channel.NewMessage(int32(mgmt_pb.ContentType_StreamCircuitsEventType), body)
	if err := handler.ch.Send(responseMsg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending StreamMetricsEvent (%s)", err)
		handler.close()
	}
}

func (handler *CircuitsStreamHandler) close() {
	if err := handler.ch.Close(); err != nil {
		pfxlog.Logger().WithError(err).Error("unexpected error closing mgmt channel")
	}
}

func NewPath(path *event.CircuitPath) *mgmt_pb.Path {
	mgmtPath := &mgmt_pb.Path{}
	mgmtPath.Nodes = append(mgmtPath.Nodes, path.Nodes...)
	mgmtPath.Links = append(mgmtPath.Links, path.Links...)
	mgmtPath.TerminatorLocalAddress = path.TerminatorLocalAddr
	return mgmtPath
}
