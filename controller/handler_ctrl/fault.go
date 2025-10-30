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
	"errors"
	"strings"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type faultHandler struct {
	r       *model.Router
	network *network.Network
}

func newFaultHandler(r *model.Router, network *network.Network) *faultHandler {
	return &faultHandler{r: r, network: network}
}

func (h *faultHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_FaultType)
}

func (h *faultHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	fault := &ctrl_pb.Fault{}
	if err := proto.Unmarshal(msg.Body, fault); err != nil {
		log.WithError(err).Error("failed to unmarshal fault message")
		return
	}

	go h.handleFault(msg, ch, fault)
}

func (h *faultHandler) handleFault(_ *channel.Message, ch channel.Channel, fault *ctrl_pb.Fault) {
	log := pfxlog.ContextLogger(ch.Label()).WithField("routerId", h.r.Id)

	switch fault.Subject {
	case ctrl_pb.FaultSubject_LinkFault:
		h.handleFaultedLink(log, fault)
	case ctrl_pb.FaultSubject_LinkDuplicate:
		h.handleFaultedLink(log, fault)

	case ctrl_pb.FaultSubject_IngressFault:
		if err := h.network.RemoveCircuit(fault.Id, false); err != nil {
			invalidCircuitErr := network.InvalidCircuitError{}
			if errors.As(err, &invalidCircuitErr) {
				log.Debugf("error handling ingress fault (%s)", err)
			} else {
				log.Errorf("error handling ingress fault (%s)", err)
			}
		} else {
			log.Debugf("handled ingress fault for (%s)", fault.Id)
		}

	case ctrl_pb.FaultSubject_EgressFault:
		if err := h.network.RemoveCircuit(fault.Id, false); err != nil {
			invalidCircuitErr := network.InvalidCircuitError{}
			if errors.As(err, &invalidCircuitErr) {
				log.Debugf("error handling egress fault (%s)", err)
			} else {
				log.Errorf("error handling egress fault (%s)", err)
			}
		} else {
			log.Debugf("handled egress fault for (%s)", fault.Id)
		}

	case ctrl_pb.FaultSubject_ForwardFault:
		circuitIds := strings.Split(fault.Id, " ")
		h.network.ReportForwardingFaults(&network.ForwardingFaultReport{
			R:            h.r,
			CircuitIds:   circuitIds,
			UnknownOwner: false,
		})

	case ctrl_pb.FaultSubject_UnknownOwnerForwardFault:
		circuitIds := strings.Split(fault.Id, " ")
		h.network.ReportForwardingFaults(&network.ForwardingFaultReport{
			R:            h.r,
			CircuitIds:   circuitIds,
			UnknownOwner: true,
		})

	default:
		log.Errorf("unexpected subject (%s)", fault.Subject.String())
	}
}

func (h *faultHandler) handleFaultedLink(log *logrus.Entry, fault *ctrl_pb.Fault) {
	linkId := fault.Id
	log = log.WithField("linkId", linkId).WithField("fault.iteration", fault.Iteration)

	if link, found := h.network.GetLink(linkId); found {
		log = log.WithField("link.iteration", link.Iteration)

		if fault.Iteration != 0 && link.Iteration != 0 && fault.Iteration < link.Iteration {
			log.Info("fault reported, but iteration is older than current link, ignoring")
			return
		}

		wasConnected := link.IsUsable()
		if err := h.network.LinkFaulted(link, fault.Subject == ctrl_pb.FaultSubject_LinkDuplicate); err == nil {
			if wasConnected {
				h.network.RerouteLink(link)
			}
			otherRouter := link.Src
			if link.Src.Id == h.r.Id {
				otherRouter = link.GetDest()
			}

			if wasConnected && otherRouter != nil {
				fault.Subject = ctrl_pb.FaultSubject_LinkFault
				if ctrl := otherRouter.Control; ctrl != nil && otherRouter.Connected.Load() {
					if err := protobufs.MarshalTyped(fault).Send(ctrl); err != nil {
						log.WithField("routerId", otherRouter.Id).
							WithError(err).Error("failed to forward link fault to other router")
					}
				}
			}

			log.Info("link fault")
		} else {
			log.WithError(err).Error("error handling link fault")
		}
	} else {
		log.Info("link fault for unknown link")
		h.network.NotifyLinkIdEvent(linkId, event.LinkFault)
	}
}
