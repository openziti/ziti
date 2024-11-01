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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/event"
	"google.golang.org/protobuf/proto"
	"slices"
	"sync"
	"time"
)

type connectEventsHandler struct {
	appEnv *env.AppEnv
	sync.Mutex
}

func NewConnectEventsHandler(appEnv *env.AppEnv) channel.TypedReceiveHandler {
	return &connectEventsHandler{
		appEnv: appEnv,
	}
}

func (h *connectEventsHandler) ContentType() int32 {
	return int32(edge_ctrl_pb.ContentType_ConnectEventsTypes)
}

func (h *connectEventsHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	go func() {
		// process per-router events in order
		h.Lock()
		defer h.Unlock()

		req := &edge_ctrl_pb.ConnectEvents{}
		routerId := ch.Id()
		if err := proto.Unmarshal(msg.Body, req); err != nil {
			pfxlog.Logger().WithError(err).Error("could not convert message to ConnectEvents")
		}

		identityManager := h.appEnv.Managers.Identity

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
					DstId:     routerId,
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
			h.appEnv.GetEventDispatcher().AcceptConnectEvent(evt)
		}
	}()
}
