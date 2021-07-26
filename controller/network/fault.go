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

package network

import (
	"github.com/sirupsen/logrus"
)

type ForwardingFaultReport struct {
	R          *Router
	CircuitIds []string
}

func (network *Network) fault(ffr *ForwardingFaultReport) {
	logrus.Infof("network fault processing for [%d] circuits", len(ffr.CircuitIds))
	for _, circuitId := range ffr.CircuitIds {
		s, found := network.circuitController.get(circuitId)
		if found {
			if err := network.rerouteCircuit(s); err == nil {
				logrus.Infof("rerouted [s/%s] in response to forwarding fault from [r/%s]", circuitId, ffr.R.Id)
			} else {
				logrus.Infof("error rerouting [s/%s] in response to forwarding fault from [r/%s] (should remove circuit?! probably not reachable...)", circuitId, ffr.R.Id)
			}

		} else {
			// unroute non-existent circuit
			if err := sendUnroute(ffr.R, circuitId, true); err == nil {
				logrus.Infof("sent unroute for [s/%s] to [r/%s] in response to forwarding fault", circuitId, ffr.R.Id)
			} else {
				logrus.Errorf("error sending unroute for [s/%s] to [r/%s] in response to forwarding fault (%v)", circuitId, ffr.R.Id, err)
			}
		}
	}
}
