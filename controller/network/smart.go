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
	"github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/model"
	"sort"
	"time"
)

func (network *Network) smart() {
	log := pfxlog.Logger()
	log.Trace("smart network processing")

	candidates := network.getRerouteCandidates()

	for _, update := range candidates {
		if retry := network.smartReroute(update.circuit, update.path, time.Now().Add(config.DefaultOptionsRouteTimeout)); retry {
			go network.rerouteCircuitWithTries(update.circuit, config.DefaultOptionsCreateCircuitRetries)
		}
	}
}

func (network *Network) calculateCircuitCost(path *model.Path) int64 {
	var cost int64
	for _, l := range path.Links {
		cost += l.GetCost()
	}
	for _, cachedRouter := range path.Nodes {
		if currentRouter := network.GetConnectedRouter(cachedRouter.Id); currentRouter != nil {
			cost += int64(max(currentRouter.Cost, network.options.MinRouterCost))
		} else {
			cost += int64(max(cachedRouter.Cost, network.options.MinRouterCost))
		}
	}
	return cost
}

func (network *Network) getRerouteCandidates() []*newCircuitPath {
	log := pfxlog.Logger()

	/*
	 * Order circuits in decreasing overall latency order
	 */
	circuits := network.GetAllCircuits()
	if len(circuits) > 0 {
		log.Debugf("observing [%d] circuits", len(circuits))
	} else {
		log.Tracef("observing [%d] circuits", len(circuits))
	}

	minRouterCost := network.options.MinRouterCost

	circuitCosts := make(map[string]int64)
	var orderedCircuits []string
	for _, circuit := range circuits {
		circuitCosts[circuit.Id] = network.calculateCircuitCost(circuit.Path)
		orderedCircuits = append(orderedCircuits, circuit.Id)
	}

	sort.SliceStable(orderedCircuits, func(i, j int) bool {
		iId := orderedCircuits[i]
		jId := orderedCircuits[j]
		return circuitCosts[jId] < circuitCosts[iId]
	})
	/* */

	/*
	 * Develop candidates for rerouting.
	 */
	var candidates []*newCircuitPath
	count := 0
	ceiling := int(float32(len(circuits)) * network.options.Smart.RerouteFraction)
	if ceiling < 1 {
		ceiling = 1
	}
	if ceiling > int(network.options.Smart.RerouteCap) {
		ceiling = int(network.options.Smart.RerouteCap)
	}
	log.Tracef("smart reroute ceiling [%d]", ceiling)
	for _, circuitId := range orderedCircuits {
		if circuit, found := network.GetCircuit(circuitId); found {
			if updatedPath, err := network.UpdatePath(circuit.Path); err == nil {
				pathChanged := !updatedPath.EqualPath(circuit.Path)
				oldCost := circuitCosts[circuitId]
				newCost := updatedPath.Cost(minRouterCost)
				costDelta := oldCost - newCost
				log.Tracef("old cost: %v, new cost: %v, delta: %v", oldCost, newCost, costDelta)
				if count < ceiling && pathChanged && costDelta >= int64(network.options.Smart.MinCostDelta) {
					count++
					candidates = append(candidates, &newCircuitPath{
						circuit: circuit,
						path:    updatedPath,
					})
					log.Debugf("rerouting [c/%s] [l:%d] %s ==> %s", circuit.Id, circuitCosts[circuit.Id], circuit.Path.String(), updatedPath.String())
				}
			}
		}
	}

	return candidates
}

type newCircuitPath struct {
	circuit *model.Circuit
	path    *model.Path
}
