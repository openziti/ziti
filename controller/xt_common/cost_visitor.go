package xt_common

import (
	"github.com/openziti/fabric/controller/xt"
	"math"
)

type CostVisitor struct {
	FailureCosts xt.FailureCosts
	CircuitCost  uint16
}

func (visitor *CostVisitor) VisitDialFailed(event xt.TerminatorEvent) {
	change := visitor.FailureCosts.Failure(event.GetTerminator().GetId())

	if change > 0 {
		xt.GlobalCosts().UpdateDynamicCost(event.GetTerminator().GetId(), func(cost uint16) uint16 {
			if cost < (math.MaxUint16 - change) {
				return cost + change
			}
			return math.MaxUint16
		})
	}
}

func (visitor *CostVisitor) VisitDialSucceeded(event xt.TerminatorEvent) {
	credit := visitor.FailureCosts.Success(event.GetTerminator().GetId())
	if credit != visitor.CircuitCost {
		xt.GlobalCosts().UpdateDynamicCost(event.GetTerminator().GetId(), func(cost uint16) uint16 {
			if visitor.CircuitCost > credit {
				increase := visitor.CircuitCost - credit
				if cost < (math.MaxUint16 - increase) {
					// pfxlog.Logger().Infof("%v: dial+ %v -> %v", event.GetTerminator().GetId(), cost, cost+increase)
					return cost + increase
				}
				// pfxlog.Logger().Infof("%v: dial+ %v -> %v", event.GetTerminator().GetId(), cost, math.MaxUint16)
				return math.MaxUint16
			}

			decrease := credit - visitor.CircuitCost
			if decrease > cost {
				// pfxlog.Logger().Infof("%v: dial+ %v -> %v", event.GetTerminator().GetId(), cost, 0)
				return 0
			}
			// pfxlog.Logger().Infof("%v: dial+ %v -> %v", event.GetTerminator().GetId(), cost, cost-decrease)
			return cost - decrease
		})
	}
}

func (visitor *CostVisitor) VisitCircuitRemoved(event xt.TerminatorEvent) {
	xt.GlobalCosts().UpdateDynamicCost(event.GetTerminator().GetId(), func(cost uint16) uint16 {
		if cost > visitor.CircuitCost {
			// pfxlog.Logger().Infof("%v: sess- %v -> %v", event.GetTerminator().GetId(), cost, cost-1)
			return cost - visitor.CircuitCost
		}
		// pfxlog.Logger().Infof("%v: sess- %v -> %v", event.GetTerminator().GetId(), cost, 0)
		return 0
	})
}
