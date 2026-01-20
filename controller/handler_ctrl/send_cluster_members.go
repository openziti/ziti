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
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
)

type sendClusterMembersHandler struct {
	baseHandler
}

func newSendClusterMembersHandler(router *model.Router, network *network.Network) *sendClusterMembersHandler {
	return &sendClusterMembersHandler{
		baseHandler: baseHandler{
			router:  router,
			network: network,
		},
	}
}

func (self *sendClusterMembersHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_RequestClusterMembers)
}

func (self *sendClusterMembersHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	go func() {
		index, data := self.network.Dispatcher.CtrlAddresses()
		log := pfxlog.Logger().WithFields(map[string]interface{}{
			"routerId":  self.router.Id,
			"channel":   self.router.Control.LogicalName(),
			"addresses": data,
			"index":     index,
		})

		log.Info("router requested cluster members")

		if len(data) == 0 {
			log.Error("no addresses to send")
			return
		}

		updMsg := &ctrl_pb.UpdateCtrlAddresses{
			IsLeader:  self.network.Dispatcher.IsLeader(),
			Addresses: data,
			Index:     index,
		}

		if err := protobufs.MarshalTyped(updMsg).Send(ch); err != nil {
			log.WithError(err).Error("error sending UpdateCtrlAddresses responding to router request")
		}
	}()
}
