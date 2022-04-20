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
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/events"
	"github.com/openziti/fabric/pb/mgmt_pb"
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

func (handler *streamCircuitsHandler) HandleReceive(_ *channel.Message, ch channel.Channel) {
	circuitsStreamHandler := &CircuitsStreamHandler{ch: ch}
	handler.streamHandlers = append(handler.streamHandlers, circuitsStreamHandler)
	events.AddCircuitEventHandler(circuitsStreamHandler)
}

func (handler *streamCircuitsHandler) HandleClose(channel.Channel) {
	for _, listener := range handler.streamHandlers {
		events.RemoveCircuitEventHandler(listener)
	}
}

type CircuitsStreamHandler struct {
	ch channel.Channel
}

func (handler *CircuitsStreamHandler) AcceptCircuitEvent(netEvent *network.CircuitEvent) {
	eventType := mgmt_pb.StreamCircuitEventType_CircuitCreated
	if netEvent.Type == network.CircuitUpdated {
		eventType = mgmt_pb.StreamCircuitEventType_PathUpdated
	} else if netEvent.Type == network.CircuitDeleted {
		eventType = mgmt_pb.StreamCircuitEventType_CircuitDeleted
	}

	var cts *int64
	if netEvent.CreationTimespan != nil {
		ctsv := int64(*netEvent.CreationTimespan)
		cts = &ctsv
	}

	event := &mgmt_pb.StreamCircuitsEvent{
		EventType:        eventType,
		CircuitId:        netEvent.CircuitId,
		ClientId:         netEvent.ClientId,
		ServiceId:        netEvent.ServiceId,
		CreationTimespan: cts,
		Path:             NewPath(netEvent.Path),
	}
	handler.sendEvent(event)
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
	events.RemoveCircuitEventHandler(handler)
}

func NewPath(path *network.Path) *mgmt_pb.Path {
	mgmtPath := &mgmt_pb.Path{}
	for _, r := range path.Nodes {
		mgmtPath.Nodes = append(mgmtPath.Nodes, r.Id)
	}
	for _, l := range path.Links {
		mgmtPath.Links = append(mgmtPath.Links, l.Id)
	}
	return mgmtPath
}
