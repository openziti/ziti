/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type listLinksHandler struct {
	network *network.Network
}

func newListLinksHandler(network *network.Network) *listLinksHandler {
	return &listLinksHandler{network: network}
}

func (h *listLinksHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_ListLinksRequestType)
}

func (h *listLinksHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	list := &mgmt_pb.ListLinksRequest{}
	if err := proto.Unmarshal(msg.Body, list); err != nil {
		sendFailure(msg, ch, err.Error())
		return
	}

	links := h.network.GetAllLinks()
	response := &mgmt_pb.ListLinksResponse{
		Links: make([]*mgmt_pb.Link, 0),
	}
	for _, l := range links {
		responseLink := &mgmt_pb.Link{
			Id:         l.Id.Token,
			Src:        l.Src.Id,
			Dst:        l.Dst.Id,
			State:      l.CurrentState().Mode.String(),
			Down:       l.Down,
			Cost:       int32(l.Cost),
			SrcLatency: l.SrcLatency,
			DstLatency: l.DstLatency,
		}
		response.Links = append(response.Links, responseLink)
	}

	body, err := proto.Marshal(response)
	if err != nil {
		sendFailure(msg, ch, err.Error())
		return
	}

	responseMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListLinksResponseType), body)
	responseMsg.ReplyTo(msg)
	if err := ch.Send(responseMsg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending response (%s)", err)
	}
}
