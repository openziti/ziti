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

package xt_weighted

import (
	"github.com/netfoundry/ziti-fabric/controller/xt"
	"math"
	"math/rand"
)

/**
The weighted strategy does random selection of available strategies in proportion to the terminator costs. So if a
given terminator has twice the fully evaluated cost as another terminator it should idealy be selected roughly half
as often.
*/

func NewFactory() xt.Factory {
	return factory{}
}

type factory struct{}

func (f factory) GetStrategyName() string {
	return "weighted"
}

func (f factory) NewStrategy() xt.Strategy {
	return strategy{}
}

type strategy struct {
	xt.DefaultEventVisitor
}

func (s strategy) Select(terminators []xt.WeightedTerminator, totalWeight uint32) (xt.Terminator, error) {
	if len(terminators) == 1 {
		return terminators[0], nil
	}
	selected := uint32(rand.Int31n(int32(totalWeight)))
	currentWeight := uint32(0)
	for _, terminator := range terminators {
		currentWeight += terminator.GetRouteWeight()
		if selected <= currentWeight {
			return terminator, nil
		}
	}
	return terminators[0], nil
}

func (s strategy) NotifyEvent(event xt.TerminatorEvent) {
	event.Accept(s)
}

func (s strategy) VisitDialFailed(event xt.TerminatorEvent) {
	weights := xt.GlobalCosts()
	weight := weights.GetPrecedenceCost(event.GetTerminator().GetId())
	if weight > 0 {
		nextWeight := int(weight) + 20
		if nextWeight < 0 {
			nextWeight = 0
		}
		weights.SetPrecedenceCost(event.GetTerminator().GetId(), uint8(nextWeight))
	}
}

func (s strategy) VisitDialSucceeded(event xt.TerminatorEvent) {
	weights := xt.GlobalCosts()
	weight := weights.GetPrecedenceCost(event.GetTerminator().GetId())
	if weight < math.MaxUint8 {
		weights.SetPrecedenceCost(event.GetTerminator().GetId(), weight-1)
	}
}

func (s strategy) HandleTerminatorChange(xt.StrategyChangeEvent) error {
	return nil
}
