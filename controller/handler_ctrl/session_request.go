/*
	Copyright 2020 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/ctrl_msg"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"time"
)

type sessionRequestHandler struct {
	r       *network.Router
	network *network.Network
}

func newSessionRequestHandler(r *network.Router, network *network.Network) *sessionRequestHandler {
	return &sessionRequestHandler{r: r, network: network}
}

func (h *sessionRequestHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_SessionRequestType)
}

func (h *sessionRequestHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	request := &ctrl_pb.SessionRequest{}
	if err := proto.Unmarshal(msg.Body, request); err == nil {
		/*
		 * This is running in a goroutine because CreateSession does a 'SendAndWait', which cannot be invoked from
		 * inside a ReceiveHandler (without parallel support).
		 */
		go func() {
			id := &identity.TokenId{Token: request.IngressId, Data: request.PeerData}
			if session, err := h.network.CreateSession(h.r, id, request.ServiceId); err == nil {
				responseMsg := ctrl_msg.NewSessionSuccessMsg(session.Id.Token, session.Circuit.IngressId)
				responseMsg.ReplyTo(msg)
				for k, v := range session.Terminator.PeerData {
					responseMsg.Headers[int32(k)] = v
				}
				if startXgressSession, err := h.r.Control.SendAndWaitWithTimeout(responseMsg, time.Second*10); err != nil {
					log.Errorf("unable to respond (%s)", err)
					h.network.RemoveSession(session.Id, true)
				} else {
					if startXgressSession.ContentType == int32(ctrl_pb.ContentType_StartXgressType) {
						if err = h.network.StartSessionEgress(session.Id); err != nil {
							log.WithError(err).Error("failure starting xgress")
							h.network.RemoveSession(session.Id, true)
						}
					} else {
						log.Errorf("unexpected reply to dial response: %v", startXgressSession.ContentType)
						h.network.RemoveSession(session.Id, true)
					}
				}
			} else {
				responseMsg := ctrl_msg.NewSessionFailedMsg(err.Error())
				responseMsg.ReplyTo(msg)
				if err := h.r.Control.Send(responseMsg); err != nil {
					log.Errorf("unable to respond (%s)", err)
				}
			}
		}()
		/* */

	} else {
		log.Errorf("unexpected error (%s)", err)
	}
}
