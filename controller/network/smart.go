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
	"sort"
	"time"

	"github.com/michaelquigley/pfxlog"
)

func (network *Network) smart() {
	log := pfxlog.Logger()
	log.Trace("smart network processing")

	/*
	 * Order circuits in decreasing overall latency order
	 */
	circuits := network.GetAllCircuits()
	if len(circuits) > 0 {
		log.Debugf("observing [%d] circuits", len(circuits))
	} else {
		log.Tracef("observing [%d] circuits", len(circuits))
	}

	circuitLatencies := make(map[string]int64)
	var orderedCircuits []string
	for _, s := range circuits {
		circuitLatencies[s.Id] = s.latency()
		orderedCircuits = append(orderedCircuits, s.Id)
	}

	sort.SliceStable(orderedCircuits, func(i, j int) bool {
		iId := orderedCircuits[i]
		jId := orderedCircuits[j]
		return circuitLatencies[jId] < circuitLatencies[iId]
	})
	/* */

	/*
	 * Develop candidates for rerouting.
	 */
	newPaths := make(map[*Circuit]*Path)
	var candidates []*Circuit
	count := 0
	ceiling := int(float32(len(circuits)) * network.options.Smart.RerouteFraction)
	if ceiling < 1 {
		ceiling = 1
	}
	if ceiling > int(network.options.Smart.RerouteCap) {
		ceiling = int(network.options.Smart.RerouteCap)
	}
	log.Tracef("smart reroute ceiling [%d]", ceiling)
	for _, sId := range orderedCircuits {
		if circuit, found := network.GetCircuit(sId); found {
			if updatedPath, err := network.UpdatePath(circuit.Path); err == nil {
				if !updatedPath.EqualPath(circuit.Path) {
					if count < ceiling {
						count++
						candidates = append(candidates, circuit)
						newPaths[circuit] = updatedPath
						log.Debugf("rerouting [s/%s] [l:%d] %s ==> %s", circuit.Id, circuitLatencies[circuit.Id], circuit.Path.String(), updatedPath.String())
					}
				}
			}
		}
	}
	/* */

	/*
	 * Reroute.
	 */
	for _, circuit := range candidates {
		if err := network.smartReroute(circuit, newPaths[circuit], time.Now().Add(DefaultNetworkOptionsRouteTimeout)); err != nil {
			log.Errorf("error rerouting [s/%s] (%s)", circuit.Id, err)
		}
	}
	/* */
}
