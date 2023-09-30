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

package network

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
)

type ForwardingFaultReport struct {
	R            *Router
	CircuitIds   []string
	UnknownOwner bool
}

func (network *Network) fault(ffr *ForwardingFaultReport) {
	logrus.Infof("network fault processing for [%d] circuits", len(ffr.CircuitIds))
	for _, circuitId := range ffr.CircuitIds {
		log := pfxlog.Logger().WithField("circuitId", circuitId).WithField("routerId", ffr.R.Id)
		s, found := network.circuitController.get(circuitId)
		if found {
			if success := network.rerouteCircuitWithTries(s, 2); success {
				log.Info("rerouted circuit in response to forwarding fault from router")
			} else {
				log.Infof("error rerouting circuit in response to forwarding fault from router, circuit removed")
			}
		} else if !ffr.UnknownOwner {
			// If the owner is unknown, we can't unroute because the circuit may be owned by some other controller
			// UnknownOwner faults come from a link forwarding operation and are generally caused by a missing forward
			// table entry. In that case there's nothing to unroute anyway.

			// unroute non-existent circuit
			if err := sendUnroute(ffr.R, circuitId, true); err == nil {
				log.Info("sent unroute for circuit to router in response to forwarding fault")
			} else {
				log.WithError(err).Error("error sending unroute for circuit to router in response to forwarding fault")
			}
		}
	}
}
