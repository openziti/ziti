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
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/handler_common"
	"github.com/openziti/fabric/pb/ctrl_pb"
)

type quiesceRouterHandler struct {
	baseHandler
}

func newQuiesceRouterHandler(router *network.Router, network *network.Network) *quiesceRouterHandler {
	return &quiesceRouterHandler{
		baseHandler: baseHandler{
			router:  router,
			network: network,
		},
	}
}

func (self *quiesceRouterHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_QuiesceRouterRequestType)
}

func (self *quiesceRouterHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).Entry
	log = log.WithField("routerId", self.router.Id)

	go func() {
		if err := self.network.Routers.QuiesceRouter(self.router, self.newChangeContext(ch, "quiesce.router")); err == nil {
			handler_common.SendSuccess(msg, ch, "router quiesced")
			log.Debug("router quiesce successful")
		} else {
			handler_common.SendFailure(msg, ch, err.Error())
			log.WithError(err).Error("router quiesce failed")
		}
	}()
}
