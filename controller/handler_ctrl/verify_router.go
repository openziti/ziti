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
	"github.com/openziti/fabric/common/handler_common"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"google.golang.org/protobuf/proto"
)

type verifyRouterHandler struct {
	r       *network.Router
	network *network.Network
}

func newVerifyRouterHandler(r *network.Router, network *network.Network) *verifyRouterHandler {
	return &verifyRouterHandler{r: r, network: network}
}

func (h *verifyRouterHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_VerifyRouterType)
}

func (h *verifyRouterHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).Entry

	verifyRouter := &ctrl_pb.VerifyRouter{}
	if err := proto.Unmarshal(msg.Body, verifyRouter); err != nil {
		log.WithError(err).Error("failed to unmarshal verify link message")
		return
	}

	log = log.WithField("routerId", verifyRouter.RouterId)

	if err := h.network.VerifyRouter(verifyRouter.RouterId, verifyRouter.Fingerprints); err == nil {
		go handler_common.SendSuccess(msg, ch, "router verified")
		log.Debug("router verification successful")
	} else {
		go handler_common.SendFailure(msg, ch, err.Error())
		log.WithError(err).Error("router verification failed")
	}
}
