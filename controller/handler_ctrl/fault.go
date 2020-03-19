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
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
)

type faultHandler struct {
	r       *network.Router
	network *network.Network
}

func newFaultHandler(r *network.Router, network *network.Network) *faultHandler {
	return &faultHandler{r: r, network: network}
}

func (h *faultHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_FaultType)
}

func (h *faultHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	fault := &ctrl_pb.Fault{}
	if err := proto.Unmarshal(msg.Body, fault); err == nil {
		switch fault.Subject {
		case ctrl_pb.FaultSubject_LinkFault:
			linkId := &identity.TokenId{Token: fault.Id}
			if err := h.network.LinkConnected(linkId, false); err == nil {
				if link, found := h.network.GetLink(linkId); found {
					h.network.LinkChanged(link)
				}
				log.Infof("link fault [l/%s]", linkId.Token)
			}

		case ctrl_pb.FaultSubject_IngressFault:
			if err := h.network.RemoveSession(&identity.TokenId{Token: fault.Id}, false); err != nil {
				log.Errorf("error handling ingress fault (%s)", err)
			} else {
				log.Debugf("handled ingress fault for (%s)", fault.Id)
			}

		case ctrl_pb.FaultSubject_EgressFault:
			if err := h.network.RemoveSession(&identity.TokenId{Token: fault.Id}, false); err != nil {
				log.Errorf("error handling egress fault (%s)", err)
			} else {
				log.Debugf("handled egress fault for (%s)", fault.Id)
			}

		default:
			log.Errorf("unexpected subject (%s)", fault.Subject.String())
		}
	} else {
		log.Errorf("unexpected error (%s)", err)
	}
}
