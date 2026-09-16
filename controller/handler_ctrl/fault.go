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
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
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

		// A link's fate is its endpoints' to report. A fault removes the link on every controller, whether
		// it propagates as a tombstone or the reporting router broadcasts it, so without this check one
		// router could evict arbitrary links network-wide.
		if !link.HasEndpoint(h.r.Id) {
			srcId := ""
			if src := link.GetSrc(); src != nil {
				srcId = src.Id
			}
			log.WithField("link.srcRouterId", srcId).
				WithField("link.destRouterId", link.DstId).
				Warn("router reported a fault for a link it is not an endpoint of, ignoring")
			return
		}

		if fault.Iteration != 0 && link.Iteration != 0 && fault.Iteration < link.Iteration {
			log.Info("fault reported, but iteration is older than current link, ignoring")
			return
		}

		if h.network.IsLinkInGossip(link) {
			// Tombstone the link in the gossip store. The tombstone propagates to
			// every controller, and each controller forwards a LinkFault to whichever
			// endpoint router is connected to it (see linkGossipListener.EntryRemoved).
			// This handles the HA case where the link's peer is connected to a
			// different controller than the one that received this fault.
			h.network.LinkFaultedViaGossip(link)
			log.Info("link fault")
			return
		}

		// A link a router reported without gossip is removed the way it was before gossip existed. That
		// router broadcasts its faults to every controller it is connected to, so each removes the link on
		// hearing it directly, and the forward below reaches a peer only if it is connected here. Kept close
		// to how it stood in 2.0: this is the path a 2.0 router still takes, so gratuitous change here would
		// be indistinguishable from a genuine incompatibility when the two versions are tested together.
		wasConnected := link.IsUsable()
		h.network.LinkFaulted(link, fault.Subject == ctrl_pb.FaultSubject_LinkDuplicate)

		otherRouter := link.GetSrc()
		if otherRouter != nil && otherRouter.Id == h.r.Id {
			otherRouter = link.GetDest()
		}

		if wasConnected && otherRouter != nil {
			fault.Subject = ctrl_pb.FaultSubject_LinkFault
			if ctrl := otherRouter.Control; ctrl != nil && otherRouter.Connected.Load() {
				// Bounded, unlike 2.0. This runs on a goroutine spawned per fault message, so an unbounded
				// send parks one goroutine per fault forwarded to a stalled router, for as long as it
				// stalls. The wait is bounded rather than the send being queued and droppable, so a healthy
				// peer is still delivered to exactly as before.
				envelope := protobufs.MarshalTyped(fault).WithTimeout(network.ForwardLinkFaultTimeout)
				if err := envelope.Send(ctrl.GetDefaultSender()); err != nil {
					log.WithField("routerId", otherRouter.Id).
						WithError(err).Error("failed to forward link fault to other router")
				}
			}
		}

		log.Info("link fault")
	} else {
		log.Info("link fault for unknown link")
		h.network.NotifyLinkIdEvent(linkId, event.LinkFault)
	}
}
