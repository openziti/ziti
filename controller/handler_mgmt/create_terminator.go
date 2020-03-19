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
	"github.com/netfoundry/ziti-fabric/controller/handler_common"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/pkg/errors"
)

type createTerminatorHandler struct {
	network *network.Network
}

func newCreateTerminatorHandler(network *network.Network) *createTerminatorHandler {
	return &createTerminatorHandler{network: network}
}

func (h *createTerminatorHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_CreateTerminatorRequestType)
}

func (h *createTerminatorHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	cs := &mgmt_pb.CreateTerminatorRequest{}
	if err := proto.Unmarshal(msg.Body, cs); err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}
	terminator, err := toModelTerminator(h.network, cs.Terminator)
	if err != nil {
		handler_common.SendFailure(msg, ch, err.Error())
		return
	}
	if id, err := h.network.Terminators.Create(terminator); err == nil {
		handler_common.SendSuccess(msg, ch, id)
	} else {
		handler_common.SendFailure(msg, ch, err.Error())
	}
}

func toModelTerminator(n *network.Network, e *mgmt_pb.Terminator) (*network.Terminator, error) {
	router, _ := n.GetRouter(e.RouterId)
	if router == nil {
		return nil, errors.Errorf("invalid router id %v", e.RouterId)
	}

	binding := "transport"
	if e.Binding != "" {
		binding = e.Binding
	}

	return &network.Terminator{
		BaseEntity: models.BaseEntity{
			Id:   e.Id,
			Tags: nil,
		},
		Service:  e.ServiceId,
		Router:   router.Id,
		Binding:  binding,
		Address:  e.Address,
		PeerData: e.PeerData,
	}, nil
}
