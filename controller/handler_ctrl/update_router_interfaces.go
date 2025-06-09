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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/handler_common"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"google.golang.org/protobuf/proto"
)

type updateRouterInterfacesHandler struct {
	baseHandler
}

func newUpdateRouterInterfacesHandler(r *model.Router, network *network.Network) *updateRouterInterfacesHandler {
	return &updateRouterInterfacesHandler{
		baseHandler: baseHandler{
			router:  r,
			network: network,
		},
	}
}

func (self *updateRouterInterfacesHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_UpdateRouterInterfaces)
}

func (self *updateRouterInterfacesHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).Entry

	request := &ctrl_pb.RouterInterfacesUpdate{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		log.WithError(err).Error("could not unmarshal update router interfaces request")
		return
	}

	go self.updateRouterInterfaces(msg, ch, request)
}

func (self *updateRouterInterfacesHandler) updateRouterInterfaces(msg *channel.Message, ch channel.Channel, request *ctrl_pb.RouterInterfacesUpdate) {
	log := pfxlog.Logger().WithField("routerId", self.router.Id)

	var interfaces []*model.Interface
	for _, intf := range request.Interfaces {
		interfaces = append(interfaces, &model.Interface{
			Name:            intf.Name,
			HardwareAddress: intf.HardwareAddress,
			MTU:             intf.Mtu,
			Index:           intf.Index,
			Flags:           intf.Flags,
			Addresses:       intf.Addresses,
		})
	}

	ctx := self.newChangeContext(ch, "fabric.create.terminator")
	err := self.network.Managers.Router.UpdateRouterInterfaces(self.router.Id, interfaces, ctx)
	if err != nil {
		log.WithError(err).Error("could not update router interfaces")
		handler_common.SendFailure(msg, ch, fmt.Errorf("failed to update router network interfaces (%w)", err).Error())
	} else {
		log.Debug("router network interfaces updated")
		handler_common.SendSuccess(msg, ch, "success")
	}
}
