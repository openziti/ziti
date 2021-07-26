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
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
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
				responseMsg := ctrl_msg.NewSessionSuccessMsg(session.Id.Token, session.Path.IngressId)
				responseMsg.ReplyTo(msg)

				//static terminator peer data
				for k, v := range session.Terminator.GetPeerData() {
					responseMsg.Headers[int32(k)] = v
				}

				//runtime peer data
				for k, v := range session.PeerData {
					responseMsg.Headers[int32(k)] = v
				}

				if err := h.r.Control.SendWithTimeout(responseMsg, time.Second*10); err != nil {
					log.Errorf("unable to respond with success to create session request for session %v (%s)", session.Id, err)
					if err := h.network.RemoveSession(session.Id, true); err != nil {
						log.Errorf("unable to remove session %v (%v)", session.Id, err)
					}
				}
			} else {
				responseMsg := ctrl_msg.NewSessionFailedMsg(err.Error())
				responseMsg.ReplyTo(msg)
				if err := h.r.Control.Send(responseMsg); err != nil {
					log.Errorf("unable to respond with failure to create session request for service %v (%s)", request.ServiceId, err)
				}
			}
		}()
		/* */

	} else {
		log.Errorf("unexpected error (%s)", err)
	}
}
