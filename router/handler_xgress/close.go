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

package handler_xgress

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xgress"
)

type closeHandler struct {
	ctrl      xgress.CtrlChannel
	forwarder *forwarder.Forwarder
}

func NewCloseHandler(ctrl xgress.CtrlChannel, forwarder *forwarder.Forwarder) *closeHandler {
	return &closeHandler{ctrl: ctrl, forwarder: forwarder}
}

func (txc *closeHandler) HandleXgressClose(x *xgress.Xgress) {
	log := pfxlog.ContextLogger(x.Label())
	log.Debug("running")
	defer log.Debug("complete")

	x.ForwardEndOfCircuit(func(payload *xgress.Payload) bool {
		log.Debug("sending end of circuit payload")
		if err := txc.forwarder.ForwardPayload(x.Address(), x.GetEndCircuit()); err != nil {
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

	log.Debug("notifying controller of fault")
	if err := protobufs.MarshalTyped(fault).Send(txc.ctrl.Channel()); err != nil {
		log.WithError(err).Error("error sending fault")
	}
}
