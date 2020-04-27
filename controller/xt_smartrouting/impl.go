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

package xt_hs_weighted

import (
	"github.com/netfoundry/ziti-fabric/controller/xt"
	"math"
)

/**
The smartrouting strategy relies purely on maninpulating costs and lets the smart routing algorithm pick the terminator.
It increases costs by a small amount when a new session uses the terminator and drops it back down when the session
finishes. It also increases the cost whenever a dial fails.
*/

func NewFactory() xt.Factory {
	return factory{}
}

type factory struct{}

func (f factory) GetStrategyName() string {
	return "smartrouting"
}

func (f factory) NewStrategy() xt.Strategy {
	return strategy{}
}

type strategy struct {
	xt.DefaultEventVisitor
}

func (s strategy) Select(terminators []xt.WeightedTerminator, totalWeight uint32) (xt.Terminator, error) {
	return terminators[0], nil
}

func (s strategy) NotifyEvent(event xt.TerminatorEvent) {
	event.Accept(s)
}

func (s strategy) VisitDialFailed(event xt.TerminatorEvent) {
	costs := xt.GlobalCosts()
	cost := costs.GetPrecedenceCost(event.GetTerminator().GetId())
	if cost > 0 {
		nextCost := int(cost) + 20
		if nextCost < 0 {
			nextCost = 0
		}
		costs.SetPrecedenceCost(event.GetTerminator().GetId(), uint8(nextCost))
	}
}

var minDialSuccessCost = uint8(math.MaxUint8 / 2)

func (s strategy) VisitDialSucceeded(event xt.TerminatorEvent) {
	costs := xt.GlobalCosts()
	cost := costs.GetPrecedenceCost(event.GetTerminator().GetId())
	if cost < math.MaxUint8 && cost < minDialSuccessCost {
		// If we have a high cost, potentially b/c of failures, don't increment
		// cost here. Cost will be reduced when sesson ends and will let us
		// shrink costs back down after dial failure
		costs.SetPrecedenceCost(event.GetTerminator().GetId(), cost+1)
	}
}

func (s strategy) VisitSessionEnded(event xt.TerminatorEvent) {
	costs := xt.GlobalCosts()
	cost := costs.GetPrecedenceCost(event.GetTerminator().GetId())
	if cost < math.MaxUint8 {
		costs.SetPrecedenceCost(event.GetTerminator().GetId(), cost-1)
	}
}

func (s strategy) HandleTerminatorChange(xt.StrategyChangeEvent) error {
	return nil
}
