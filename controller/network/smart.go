/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"sort"
)

func (network *Network) smart() {
	log := pfxlog.Logger()
	log.Debugf("smart network processing")

	/*
	 * Order sessions in decreasing overall latency order
	 */
	sessions := network.GetAllSessions()
	log.Debugf("observing [%d] sessions", len(sessions))

	sessionLatencies := make(map[*identity.TokenId]int64)
	var orderedSessions []*identity.TokenId
	for _, s := range sessions {
		sessionLatencies[s.Id] = s.latency()
		orderedSessions = append(orderedSessions, s.Id)
	}

	sort.SliceStable(orderedSessions, func(i, j int) bool {
		iId := orderedSessions[i]
		jId := orderedSessions[j]
		return sessionLatencies[jId] < sessionLatencies[iId]
	})
	/* */

	/*
	 * Develop candidates for rerouting.
	 */
	newCircuits := make(map[*session]*Circuit)
	var candidates []*session
	count := 0
	ceiling := int(float32(len(sessions)) * network.options.Smart.RerouteFraction)
	if ceiling < 1 {
		ceiling = 1
	}
	if ceiling > int(network.options.Smart.RerouteCap) {
		ceiling = int(network.options.Smart.RerouteCap)
	}
	log.Debugf("smart reroute ceiling [%d]", ceiling)
	for _, sId := range orderedSessions {
		if session, found := network.GetSession(sId); found {
			if updatedCircuit, err := network.UpdateCircuit(session.Circuit); err == nil {
				if !updatedCircuit.EqualPath(session.Circuit) {
					if count < ceiling {
						count++
						candidates = append(candidates, session)
						newCircuits[session] = updatedCircuit
						log.Infof("rerouting [s/%s] [l:%d] %s ==> %s", session.Id.Token, sessionLatencies[session.Id], session.Circuit.String(), updatedCircuit.String())
					}
				}
			}
		}
	}
	/* */

	/*
	 * Reroute.
	 */
	for _, session := range candidates {
		if err := network.smartReroute(session, newCircuits[session]); err != nil {
			log.Errorf("error rerouting [s/%s] (%s)", session.Id.Token, err)
		}
	}
	/* */
}

