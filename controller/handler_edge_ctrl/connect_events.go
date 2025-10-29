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

package handler_edge_ctrl

import (
	"slices"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/event"
	"google.golang.org/protobuf/proto"
)

type connectEventsHandler struct {
	appEnv *env.AppEnv
	eventC chan func()
}

func NewConnectEventsHandler(appEnv *env.AppEnv) channel.TypedReceiveHandler {
	result := &connectEventsHandler{
		appEnv: appEnv,
		eventC: make(chan func(), 1000),
	}

	go result.processEvents()
	return result
}

func (self *connectEventsHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_ConnectEventsTypes)
}

func (self *connectEventsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	req := &edge_ctrl_pb.ConnectEvents{}
	if err := proto.Unmarshal(msg.Body, req); err != nil {
		pfxlog.Logger().WithError(err).Error("could not convert message to ConnectEvents")
		return
	}

	processF := func() {
		self.HandleConnectEvents(req, ch)
	}

	select {
	case self.eventC <- processF:
	case <-self.appEnv.GetCloseNotifyChannel():
	}
}

func (self *connectEventsHandler) processEvents() {
	for {
		select {
		case eventF := <-self.eventC:
			eventF()
		case <-self.appEnv.GetCloseNotifyChannel():
			return
		}
	}
}

func (self *connectEventsHandler) HandleConnectEvents(req *edge_ctrl_pb.ConnectEvents, ch channel.Channel) {
	identityManager := self.appEnv.Managers.Identity

	if req.FullState {
		identityManager.GetConnectionTracker().SyncAllFromRouter(req, ch)
	}

	var events []*event.ConnectEvent
	for _, identityEvent := range req.Events {
		for _, connect := range identityEvent.ConnectTimes {
			events = append(events, &event.ConnectEvent{
				Namespace: event.ConnectEventNS,
				SrcType:   event.ConnectSourceIdentity,
				DstType:   event.ConnectDestinationRouter,
				SrcId:     identityEvent.IdentityId,
				SrcAddr:   connect.SrcAddr,
				DstId:     ch.Id(),
				DstAddr:   connect.DstAddr,
				Timestamp: time.UnixMilli(connect.ConnectTime),
			})
		}

		if !req.FullState {
			if identityEvent.IsConnected {
				identityManager.GetConnectionTracker().MarkConnected(identityEvent.IdentityId, ch)
			} else {
				identityManager.GetConnectionTracker().MarkDisconnected(identityEvent.IdentityId, ch)
			}
		}
	}

	slices.SortFunc(events, func(a, b *event.ConnectEvent) int {
		return int(a.Timestamp.UnixMilli() - b.Timestamp.UnixMilli())
	})

	for _, evt := range events {
		self.appEnv.GetEventDispatcher().AcceptConnectEvent(evt)
	}
}
