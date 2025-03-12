package xt_common

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/controller/xt"
	cmap "github.com/orcaman/concurrent-map/v2"
	"math"
	"sync/atomic"
	"time"
)

type TerminatorCosts struct {
	CircuitCount uint32
	FailureCost  uint32
	CachedCost   uint32
	LastFailure  time.Time
}

func (self *TerminatorCosts) cache(circuitCost uint32) {
	cost := uint64(self.CircuitCount)*uint64(circuitCost) + uint64(self.FailureCost)
	if cost > math.MaxUint32 {
		cost = math.MaxUint32
	}
	atomic.StoreUint32(&self.CachedCost, uint32(cost))
}

func (self *TerminatorCosts) Get() uint16 {
	val := atomic.LoadUint32(&self.CachedCost)
	if val > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(val)
}

func (self *TerminatorCosts) Inspect(terminatorId string) *inspect.TerminatorCostDetail {
	return &inspect.TerminatorCostDetail{
		TerminatorId: terminatorId,
		CircuitCount: self.CircuitCount,
		FailureCost:  self.FailureCost,
		CurrentCost:  self.CachedCost,
	}
}

func NewCostVisitor(circuitCost, failureCost, successCredit uint16) *CostVisitor {
	return &CostVisitor{
		Costs:         cmap.New[*TerminatorCosts](),
		CircuitCost:   uint32(circuitCost),
		FailureCost:   uint32(failureCost),
		SuccessCredit: uint32(successCredit),
	}
}

type CostVisitor struct {
	Costs         cmap.ConcurrentMap[string, *TerminatorCosts]
	CircuitCost   uint32
	FailureCost   uint32
	SuccessCredit uint32
}

func (self *CostVisitor) GetFailureCost(terminatorId string) uint32 {
	val, _ := self.Costs.Get(terminatorId)
	if val == nil {
		return 0
	}
	return val.FailureCost
}

func (self *CostVisitor) GetCircuitCount(terminatorId string) uint32 {
	val, _ := self.Costs.Get(terminatorId)
	if val == nil {
		return 0
	}
	return val.CircuitCount
}

func (self *CostVisitor) GetCost(terminatorId string) uint32 {
	val, _ := self.Costs.Get(terminatorId)
	if val == nil {
		return 0
	}
	return atomic.LoadUint32(&val.CachedCost)
}

func (self *CostVisitor) VisitDialFailed(event xt.TerminatorEvent) {
	self.Costs.Upsert(event.GetTerminator().GetId(), nil, func(exist bool, valueInMap *TerminatorCosts, newValue *TerminatorCosts) *TerminatorCosts {
		cost := valueInMap
		if !exist {
			cost = &TerminatorCosts{}
			xt.GlobalCosts().SetDynamicCost(event.GetTerminator().GetId(), cost)
		}

		if math.MaxUint32-cost.FailureCost > self.FailureCost {
			cost.FailureCost += self.FailureCost
		} else {
			cost.FailureCost = math.MaxUint32
		}
		cost.cache(self.CircuitCost)
		cost.LastFailure = time.Now()
		return cost
	})
}

func (self *CostVisitor) VisitDialSucceeded(event xt.TerminatorEvent) {
	self.Costs.Upsert(event.GetTerminator().GetId(), nil, func(exist bool, valueInMap *TerminatorCosts, newValue *TerminatorCosts) *TerminatorCosts {
		cost := valueInMap
		if !exist {
			cost = &TerminatorCosts{}
			xt.GlobalCosts().SetDynamicCost(event.GetTerminator().GetId(), cost)
		}

		if cost.FailureCost > self.SuccessCredit {
			cost.FailureCost -= self.SuccessCredit
		} else {
			cost.FailureCost = 0
		}

		if cost.CircuitCount < math.MaxUint32/self.CircuitCost {
			cost.CircuitCount++
		}
		cost.cache(self.CircuitCost)
		return cost
	})
}

func (self *CostVisitor) VisitCircuitRemoved(event xt.TerminatorEvent) {
	self.Costs.Upsert(event.GetTerminator().GetId(), nil, func(exist bool, valueInMap *TerminatorCosts, newValue *TerminatorCosts) *TerminatorCosts {
		cost := valueInMap
		if !exist {
			cost = &TerminatorCosts{}
			xt.GlobalCosts().SetDynamicCost(event.GetTerminator().GetId(), cost)
		}

		if cost.CircuitCount > 0 {
			cost.CircuitCount--
		}
		cost.cache(self.CircuitCost)
		return cost
	})
}

func (self *CostVisitor) CreditOverTime(credit uint8, period time.Duration) *time.Ticker {
	ticker := time.NewTicker(period)
	go func() {
		for range ticker.C {
			self.CreditAll(credit)
		}
	}()
	return ticker
}

func (self *CostVisitor) CreditAll(credit uint8) {
	var keys []string
	self.Costs.IterCb(func(key string, _ *TerminatorCosts) {
		keys = append(keys, key)
	})

	for _, key := range keys {
		self.Costs.Upsert(key, nil, func(exist bool, valueInMap *TerminatorCosts, newValue *TerminatorCosts) *TerminatorCosts {
			cost := valueInMap
			if !exist {
				cost = &TerminatorCosts{}
				xt.GlobalCosts().SetDynamicCost(key, cost)
			}

			if cost.FailureCost > uint32(credit) {
				cost.FailureCost -= uint32(credit)
			}
			cost.cache(self.CircuitCost)
			return cost
		})
	}
}

func (self *CostVisitor) CreditOverTimeExponential(period time.Duration, exponentBasis time.Duration) *time.Ticker {
	ticker := time.NewTicker(period)
	go func() {
		for range ticker.C {
			self.CreditAllExponential(exponentBasis)
		}
	}()
	return ticker
}

func (self *CostVisitor) CreditAllExponential(exponentBasis time.Duration) {
	var keys []string
	self.Costs.IterCb(func(key string, _ *TerminatorCosts) {
		keys = append(keys, key)
	})

	for _, key := range keys {
		self.Costs.Upsert(key, nil, func(exist bool, valueInMap *TerminatorCosts, newValue *TerminatorCosts) *TerminatorCosts {
			cost := valueInMap
			if !exist {
				cost = &TerminatorCosts{}
				xt.GlobalCosts().SetDynamicCost(key, cost)
			} else if cost.FailureCost > 0 {
				credit64 := uint64(math.Pow(2, float64(time.Since(cost.LastFailure)/exponentBasis)))
				if credit64 > math.MaxUint32 {
					credit64 = math.MaxUint32
				}
				if credit64 < 10 {
					credit64 = 10
				}

				credit32 := uint32(credit64)
				if credit32 > cost.FailureCost {
					cost.FailureCost = 0
				} else {
					cost.FailureCost -= credit32
				}

				pfxlog.Logger().Tracef("credit over time, id: %s, credit: %v, new failure cost: %v", key, credit32, cost.FailureCost)
				cost.cache(self.CircuitCost)
			}
			return cost
		})
	}
}

func (self *CostVisitor) NotifyEvent(event xt.TerminatorEvent) {
	event.Accept(self)
}

func (self *CostVisitor) HandleTerminatorChange(event xt.StrategyChangeEvent) error {
	for _, t := range event.GetRemoved() {
		self.Costs.Remove(t.GetId())
	}
	return nil
}
