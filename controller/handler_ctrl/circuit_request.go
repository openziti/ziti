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
	"github.com/openziti/fabric/controller/xt"
	"google.golang.org/protobuf/proto"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/logcontext"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/identity"
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

func (h *circuitRequestHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).Entry

	request := &ctrl_pb.CircuitRequest{}
	if err := proto.Unmarshal(msg.Body, request); err == nil {
		/*
		 * This is running in a goroutine because CreateCircuit does a 'SendAndWait', which cannot be invoked from
		 * inside a ReceiveHandler (without parallel support).
		 */
		go func() {
			id := &identity.TokenId{Token: request.IngressId, Data: request.PeerData}
			service := request.Service
			if _, err := h.network.Managers.Services.Read(service); err != nil {
				if id, _ := h.network.Managers.Services.GetIdForName(service); id != "" {
					service = id
				}
			}
			log = log.WithField("serviceId", service)
			if circuit, err := h.network.CreateCircuit(h.newCircuitCreateParms(service, h.r, id)); err == nil {
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

				if err := responseMsg.WithTimeout(10 * time.Second).Send(h.r.Control); err != nil {
					log.Errorf("unable to respond with success to create circuit request for circuit %v (%s)", circuit.Id, err)
					if err := h.network.RemoveCircuit(circuit.Id, true); err != nil {
						log.WithError(err).WithField("circuitId", circuit.Id).Error("unable to remove circuit")
					}
				}
			} else {
				responseMsg := ctrl_msg.NewCircuitFailedMsg(err.Error())
				responseMsg.ReplyTo(msg)
				if err := h.r.Control.Send(responseMsg); err != nil {
					log.WithError(err).Error("unable to respond with failure to create circuit request for service")
				}
			}
		}()
		/* */

	} else {
		log.Errorf("unexpected error (%s)", err)
	}
}

func (h *circuitRequestHandler) newCircuitCreateParms(serviceId string, sourceRouter *network.Router, clientId *identity.TokenId) network.CreateCircuitParams {
	return &circuitParams{
		serviceId:    serviceId,
		sourceRouter: sourceRouter,
		clientId:     clientId,
		ctx:          logcontext.NewContext(),
		deadline:     time.Now().Add(h.network.GetOptions().RouteTimeout),
	}
}

type circuitParams struct {
	serviceId    string
	sourceRouter *network.Router
	clientId     *identity.TokenId
	ctx          logcontext.Context
	deadline     time.Time
}

func (self *circuitParams) GetServiceId() string {
	return self.serviceId
}

func (self *circuitParams) GetSourceRouter() *network.Router {
	return self.sourceRouter
}

func (self *circuitParams) GetClientId() *identity.TokenId {
	return self.clientId
}

func (self *circuitParams) GetCircuitTags(xt.CostedTerminator) map[string]string {
	return map[string]string{
		"serviceId": self.serviceId,
	}
}

func (self *circuitParams) GetLogContext() logcontext.Context {
	return self.ctx
}

func (self *circuitParams) GetDeadline() time.Time {
	return self.deadline
}
