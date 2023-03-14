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

package xt

import (
	cmap "github.com/orcaman/concurrent-map/v2"
	"math"
)

const (
	unknownMinCost = math.MaxUint16 * 12
	failedMinCost  = math.MaxUint16 * 8
	defaultMinCost = math.MaxUint16 * 4
	requireMinCost = 0
)

var globalCosts = &costs{
	costMap: cmap.New[uint16](),
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

func (p *precedence) GetBiasedCost(cost uint32) uint32 {
	result := p.getMinCost() + cost
	if result > p.maxCost {
		return p.maxCost
	}
	return result
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
		maxCost: defaultMinCost - 1,
	},
	Default: &precedence{
		name:    "default",
		minCost: defaultMinCost,
		maxCost: failedMinCost - 1,
	},
	Failed: &precedence{
		name:    "failed",
		minCost: failedMinCost,
		maxCost: unknownMinCost - 1,
	},
	unknown: &precedence{
		name:    "unknown",
		minCost: unknownMinCost,
		maxCost: unknownMinCost + (math.MaxUint16 * 4) - 1,
	},
}

func GetPrecedenceForName(name string) Precedence {
	if Precedences.Required.String() == name {
		return Precedences.Required
	}
	if Precedences.Failed.String() == name {
		return Precedences.Failed
	}
	return Precedences.Default
}

type costs struct {
	costMap cmap.ConcurrentMap[string, uint16]
}

func (self *costs) ClearCost(terminatorId string) {
	self.costMap.Remove(terminatorId)
}

func (self *costs) SetDynamicCost(terminatorId string, cost uint16) {
	self.costMap.Set(terminatorId, cost)
}

func (self *costs) UpdateDynamicCost(terminatorId string, updateF func(uint16) uint16) {
	self.costMap.Upsert(terminatorId, 0, func(exist bool, valueInMap uint16, newValue uint16) uint16 {
		if !exist {
			return updateF(0)
		}

		return updateF(valueInMap)
	})
}

func (self *costs) GetDynamicCost(terminatorId string) uint16 {
	if cost, found := self.costMap.Get(terminatorId); found {
		return cost
	}
	return 0
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
