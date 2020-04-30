package xt_common

import (
	"github.com/netfoundry/ziti-fabric/controller/xt"
	"math"
)

type CostVisitor struct {
	FailureCosts xt.FailureCosts
	SessionCost  uint16
}

func (visitor *CostVisitor) VisitDialFailed(event xt.TerminatorEvent) {
	change := visitor.FailureCosts.Failure(event.GetTerminator().GetId())

	if change > 0 {
		xt.GlobalCosts().UpdatePrecedenceCost(event.GetTerminator().GetId(), func(cost uint16) uint16 {
			if cost < (math.MaxUint16 - change) {
				return cost + change
			}
			return math.MaxUint16
		})
	}
}

func (visitor *CostVisitor) VisitDialSucceeded(event xt.TerminatorEvent) {
	credit := visitor.FailureCosts.Success(event.GetTerminator().GetId())
	if credit != visitor.SessionCost {
		xt.GlobalCosts().UpdatePrecedenceCost(event.GetTerminator().GetId(), func(cost uint16) uint16 {
			if visitor.SessionCost > credit {
				increase := visitor.SessionCost - credit
				if cost < (math.MaxUint16 - increase) {
					return cost + increase
				}
				return math.MaxUint16
			}

			decrease := credit - visitor.SessionCost
			if decrease > cost {
				return 0
			}
			return cost - decrease
		})
	}
}

func (visitor *CostVisitor) VisitSessionEnded(event xt.TerminatorEvent) {
	xt.GlobalCosts().UpdatePrecedenceCost(event.GetTerminator().GetId(), func(cost uint16) uint16 {
		if cost > 0 {
			return cost - 1
		}
		return 0
	})
}
