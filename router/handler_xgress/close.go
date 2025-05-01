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

package handler_xgress

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/forwarder"
	"github.com/openziti/sdk-golang/xgress"
	"time"
)

type closeHandler struct {
	ctrls     env.NetworkControllers
	forwarder *forwarder.Forwarder
}

func NewCloseHandler(ctrl env.NetworkControllers, forwarder *forwarder.Forwarder) xgress.CloseHandler {
	return &closeHandler{ctrls: ctrl, forwarder: forwarder}
}

func (txc *closeHandler) HandleXgressClose(x *xgress.Xgress) {
	log := pfxlog.ContextLogger(x.Label())
	log.Debug("running")
	defer log.Debug("complete")

	x.ForwardEndOfCircuit(func(payload *xgress.Payload) bool {
		log.Debug("sending end of circuit payload")
		if err := txc.forwarder.ForwardPayload(x.Address(), payload, time.Second); err != nil {
			// ok that we couldn't forward close, as that means it was already closed
			log.Debugf("error forwarding end circuit payload (%s)", err)
			return false
		}
		return true
	})

	// Notify the forwarder that the circuit is ending
	log.Debug("removing circuit from forwarder")
	txc.forwarder.EndCircuit(x.CircuitId())

	// Notify the controller of the xgress fault
	fault := &ctrl_pb.Fault{Id: x.CircuitId()}
	if x.Originator() == xgress.Initiator {
		fault.Subject = ctrl_pb.FaultSubject_IngressFault
	} else if x.Originator() == xgress.Terminator {
		fault.Subject = ctrl_pb.FaultSubject_EgressFault
	}

	ch := txc.ctrls.GetCtrlChannel(x.CtrlId())
	if ch == nil {
		log.WithField("ctrlId", x.CtrlId()).Error("control channel not available")
	} else {
		log.Debug("notifying controller of fault")
		if err := protobufs.MarshalTyped(fault).Send(ch); err != nil {
			log.WithError(err).Error("error sending fault")
		}
	}
}
