package xt

import (
	cmap "github.com/orcaman/concurrent-map"
	"time"
)

type failureCosts struct {
	costMap        cmap.ConcurrentMap
	maxFailureCost uint32
	failureCost    uint16
	successCredit  uint16
}

func NewFailureCosts(maxFailureCost uint16, failureCost uint8, successCredit uint8) FailureCosts {
	result := &failureCosts{
		costMap:        cmap.New(),
		maxFailureCost: uint32(maxFailureCost),
		failureCost:    uint16(failureCost),
		successCredit:  uint16(successCredit),
	}

	return result
}

func (self *failureCosts) CreditOverTime(credit uint8, period time.Duration) *time.Ticker {
	ticker := time.NewTicker(period)
	go func() {
		for range ticker.C {
			for val := range self.costMap.IterBuffered() {
				self.successWithCredit(val.Key, uint16(credit))
			}
		}
	}()
	return ticker
}

func (self *failureCosts) Clear(terminatorId string) {
	self.costMap.Remove(terminatorId)
}

func (self *failureCosts) Failure(terminatorId string) uint16 {
	var change uint16
	self.costMap.Upsert(terminatorId, nil, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		if !exist {
			change = self.failureCost
			return self.failureCost
		}
		currentCost := valueInMap.(uint16)
		nextCost := uint32(currentCost) + uint32(self.failureCost)
		if nextCost > self.maxFailureCost {
			change = uint16(self.maxFailureCost - uint32(currentCost))
			return uint16(self.maxFailureCost)
		}
		change = self.failureCost
		return uint16(nextCost)
	})
	return change
}

func (self *failureCosts) Success(terminatorId string) uint16 {
	return self.successWithCredit(terminatorId, self.successCredit)
}

func (self *failureCosts) successWithCredit(terminatorId string, credit uint16) uint16 {
	val, found := self.costMap.Get(terminatorId)
	if !found {
		return 0
	}

	if val == 0 {
		removed := self.costMap.RemoveCb(terminatorId, func(key string, v interface{}, exists bool) bool {
			if !exists {
				return true
			}
			currentVal := v.(uint16)
			return currentVal == 0
		})
		if removed {
			return 0
		}
	}

	var change uint16
	self.costMap.Upsert(terminatorId, nil, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		if !exist {
			change = 0
			return 0
		}
		currentCost := valueInMap.(uint16)
		if currentCost < credit {
			change = currentCost
			return 0
		}
		change = credit
		return currentCost - credit
	})
	return change
}
