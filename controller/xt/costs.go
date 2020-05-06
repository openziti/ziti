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

package xt

import (
	cmap "github.com/orcaman/concurrent-map"
	"math"
)

const (
	unknownMinCost = math.MaxUint16 * 32
	failedMinCost  = math.MaxUint16 * 8
	defaultMinCost = math.MaxUint16 * 4
	requireMinCost = 0
)

var globalCosts = &costs{
	costMap: cmap.New(),
}

func GlobalCosts() Costs {
	return globalCosts
}

type precedence struct {
	name    string
	minCost uint32
	maxCost uint32
}

func (p *precedence) String() string {
	return p.name
}

func (p *precedence) Unbias(cost uint32) uint32 {
	return cost - p.minCost
}

func (p *precedence) IsFailed() bool {
	return p.minCost == failedMinCost
}

func (p *precedence) IsDefault() bool {
	return p.minCost == defaultMinCost
}

func (p *precedence) IsRequired() bool {
	return p.minCost == requireMinCost
}

func (p *precedence) getMinCost() uint32 {
	return p.minCost
}

func (p *precedence) getMaxCost() uint32 {
	return p.maxCost
}

// Precedences define the precedence levels
var Precedences = struct {
	// Required terminators should always be used in preference to non-required terminators
	// Example: The current active node in an active/passive setup. If the strategy determintes
	//          that the current active has failed, it will drop the precedence and mark some other
	//          terminator as Required
	Required Precedence

	// Default precedence is where terminators start
	Default Precedence

	// Failed means this terminator should only be used if all other terminators are also in a failed state
	// Example: A strategy might move a terminator to Failed if three dials in a row fail
	Failed Precedence

	// Unknown means this terminator was likely recently removed and should not be used
	unknown Precedence
}{
	Required: &precedence{
		name:    "required",
		minCost: requireMinCost,
		maxCost: requireMinCost + (math.MaxUint16 - 1),
	},
	Default: &precedence{
		name:    "default",
		minCost: defaultMinCost,
		maxCost: defaultMinCost + (math.MaxUint16 - 1),
	},
	Failed: &precedence{
		name:    "failed",
		minCost: failedMinCost,
		maxCost: failedMinCost + (math.MaxUint16 - 1),
	},
	unknown: &precedence{
		name:    "unknown",
		minCost: unknownMinCost,
		maxCost: unknownMinCost + (math.MaxUint16 - 1),
	},
}

type terminatorStats struct {
	cost           uint32
	precedence     Precedence
	precedenceCost uint16
}

func (stats *terminatorStats) GetCost() uint32 {
	return stats.cost
}

func (stats *terminatorStats) GetPrecedence() Precedence {
	return stats.precedence
}

type costs struct {
	costMap cmap.ConcurrentMap
}

func (self *costs) ClearCost(terminatorId string) {
	self.costMap.Remove(terminatorId)
}

func (self *costs) GetCost(terminatorId string) uint32 {
	stats := self.getStats(terminatorId)
	if stats == nil {
		return Precedences.Default.getMinCost()
	}
	return stats.cost
}

func (self *costs) GetStats(terminatorId string) Stats {
	stats := self.getStats(terminatorId)
	if stats == nil {
		return &terminatorStats{
			cost:           Precedences.unknown.getMinCost(),
			precedence:     Precedences.unknown,
			precedenceCost: 0,
		}
	}
	return stats
}

func (self *costs) getStats(terminatorId string) *terminatorStats {
	val, found := self.costMap.Get(terminatorId)
	if !found {
		return nil
	}
	return val.(*terminatorStats)
}

func (self *costs) GetPrecedence(terminatorId string) Precedence {
	stats := self.getStats(terminatorId)
	if stats == nil {
		return Precedences.Default
	}
	return stats.precedence
}

func (self *costs) SetPrecedence(terminatorId string, precedence Precedence) {
	stats := self.getStats(terminatorId)
	var precedenceCost uint16
	if stats != nil {
		precedenceCost = stats.precedenceCost
	}
	self.costMap.Set(terminatorId, self.newStats(precedence, precedenceCost))
}

func (self *costs) SetPrecedenceCost(terminatorId string, cost uint16) {
	stats := self.getStats(terminatorId)
	var p Precedence
	if stats == nil {
		p = Precedences.Default
	} else {
		p = stats.precedence
	}
	self.costMap.Set(terminatorId, self.newStats(p, cost))
}

func (self *costs) UpdatePrecedenceCost(terminatorId string, updateF func(uint16) uint16) {
	self.costMap.Upsert(terminatorId, nil, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		if !exist {
			cost := updateF(0)
			return self.newStats(Precedences.Default, cost)
		}
		stats := valueInMap.(*terminatorStats)
		return self.newStats(stats.precedence, updateF(stats.precedenceCost))
	})
}

func (self *costs) TerminatorCreated(terminatorId string) {
	self.costMap.Upsert(terminatorId, nil, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		if !exist {
			return self.newStats(Precedences.Default, 0)
		}
		return valueInMap
	})
}

func (self *costs) GetPrecedenceCost(terminatorId string) uint16 {
	stats := self.getStats(terminatorId)
	if stats == nil {
		return 0
	}
	return stats.precedenceCost
}

func (self *costs) newStats(precedence Precedence, cost uint16) *terminatorStats {
	if cost == math.MaxUint16 {
		cost--
	}

	return &terminatorStats{
		cost:           precedence.getMinCost() + uint32(cost),
		precedence:     precedence,
		precedenceCost: cost,
	}
}

// In a list which is sorted by precedence, returns the terminators which have the
// same precedence as that of the first entry in the list
func GetRelatedTerminators(list []CostedTerminator) []CostedTerminator {
	first := list[0]
	var result = []CostedTerminator{first}
	for _, t := range list[1:] {
		if t.GetPrecedence() == first.GetPrecedence() {
			result = append(result, t)
		} else {
			break
		}
	}
	return result
}
