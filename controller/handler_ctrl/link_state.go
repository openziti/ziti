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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"google.golang.org/protobuf/proto"
	"time"
)

type linkStateHandler struct {
	r       *model.Router
	network *network.Network
}

func newLinkStateHandler(r *model.Router, network *network.Network) *linkStateHandler {
	return &linkStateHandler{r: r, network: network}
}

func (h *linkStateHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_LinkState)
}

func (h *linkStateHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	link := &ctrl_pb.LinkStateUpdate{}
	if err := proto.Unmarshal(msg.Body, link); err != nil {
		log.WithError(err).Error("failed to unmarshal link state message")
		return
	}

	h.HandleLinks(link)
}

func (h *linkStateHandler) HandleLinks(update *ctrl_pb.LinkStateUpdate) {
	if link, _ := h.network.Link.Get(update.LinkId); link != nil && update.LinkIteration == link.Iteration {
		link.SetConnsState(update.ConnState)
	}

	h.network.GetEventDispatcher().AcceptLinkEvent(&event.LinkEvent{
		Namespace:  event.LinkEventNS,
		EventSrcId: h.network.GetAppId(),
		Timestamp:  time.Now(),
		EventType:  event.LinkConnectionsChanged,
		LinkId:     update.LinkId,

		Connections: func() []*event.LinkConnection {
			var result []*event.LinkConnection
			for _, c := range update.ConnState.Conns {
				result = append(result, &event.LinkConnection{
					Id:         c.Type,
					LocalAddr:  c.LocalAddr,
					RemoteAddr: c.RemoteAddr,
				})
			}
			return result
		}(),
	})
}
