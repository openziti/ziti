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
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"time"
)

type circuitRequestHandler struct {
	r       *network.Router
	network *network.Network
}

func newCircuitRequestHandler(r *network.Router, network *network.Network) *circuitRequestHandler {
	return &circuitRequestHandler{r: r, network: network}
}

func (h *circuitRequestHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_CircuitRequestType)
}

func (h *circuitRequestHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	request := &ctrl_pb.CircuitRequest{}
	if err := proto.Unmarshal(msg.Body, request); err == nil {
		/*
		 * This is running in a goroutine because CreateCircuit does a 'SendAndWait', which cannot be invoked from
		 * inside a ReceiveHandler (without parallel support).
		 */
		go func() {
			id := &identity.TokenId{Token: request.IngressId, Data: request.PeerData}
			if circuit, err := h.network.CreateCircuit(h.r, id, request.ServiceId, logcontext.NewContext()); err == nil {
				responseMsg := ctrl_msg.NewCircuitSuccessMsg(circuit.Id, circuit.Path.IngressId)
				responseMsg.ReplyTo(msg)

				//static terminator peer data
				for k, v := range circuit.Terminator.GetPeerData() {
					responseMsg.Headers[int32(k)] = v
				}

				//runtime peer data
				for k, v := range circuit.PeerData {
					responseMsg.Headers[int32(k)] = v
				}

				if err := h.r.Control.SendWithTimeout(responseMsg, time.Second*10); err != nil {
					log.Errorf("unable to respond with success to create circuit request for circuit %v (%s)", circuit.Id, err)
					if err := h.network.RemoveCircuit(circuit.Id, true); err != nil {
						log.Errorf("unable to remove circuit %v (%v)", circuit.Id, err)
					}
				}
			} else {
				responseMsg := ctrl_msg.NewCircuitFailedMsg(err.Error())
				responseMsg.ReplyTo(msg)
				if err := h.r.Control.Send(responseMsg); err != nil {
					log.Errorf("unable to respond with failure to create circuit request for service %v (%s)", request.ServiceId, err)
				}
			}
		}()
		/* */

	} else {
		log.Errorf("unexpected error (%s)", err)
	}
}
