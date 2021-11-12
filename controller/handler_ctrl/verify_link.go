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

package handler_ctrl

import (
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/handler_common"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
)

type verifyLinkHandler struct {
	r       *network.Router
	network *network.Network
}

func newVerifyLinkHandler(r *network.Router, network *network.Network) *verifyLinkHandler {
	return &verifyLinkHandler{r: r, network: network}
}

func (h *verifyLinkHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_VerifyLinkType)
}

func (h *verifyLinkHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	verifyLink := &ctrl_pb.VerifyLink{}
	if err := proto.Unmarshal(msg.Body, verifyLink); err != nil {
		log.WithError(err).Error("failed to unmarshal verify link message")
		return
	}

	if err := h.network.VerifyLinkSource(h.r, &identity.TokenId{Token: verifyLink.LinkId}, verifyLink.Fingerprints); err == nil {
		go handler_common.SendSuccess(msg, ch, "link verified")
		log.Debugf("link verification successful [l/%s]", verifyLink.LinkId)
	} else {
		go handler_common.SendFailure(msg, ch, err.Error())
		log.WithError(err).Error("link verification failed")
	}
}
