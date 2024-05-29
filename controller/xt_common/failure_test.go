package xt_common

import (
	"fmt"
	"github.com/openziti/ziti/controller/xt"
	cmap "github.com/orcaman/concurrent-map/v2"
	"math/rand"
	"sync"
	"testing"
	"time"
)

type mockTerminator struct{}

func (m mockTerminator) GetId() string {
	return "test"
}

func (m mockTerminator) GetPrecedence() xt.Precedence {
	panic("implement me")
}

func (m mockTerminator) GetCost() uint16 {
	panic("implement me")
}

func (m mockTerminator) GetServiceId() string {
	panic("implement me")
}

func (m mockTerminator) GetInstanceId() string {
	panic("implement me")
}

func (m mockTerminator) GetRouterId() string {
	panic("implement me")
}

func (m mockTerminator) GetBinding() string {
	panic("implement me")
}

func (m mockTerminator) GetAddress() string {
	panic("implement me")
}

func (m mockTerminator) GetPeerData() xt.PeerData {
	panic("implement me")
}

func (m mockTerminator) GetCreatedAt() time.Time {
	panic("implement me")
}

func (m mockTerminator) GetHostId() string {
	panic("implement me")
}

func (m mockTerminator) GetSourceCtrl() string {
	panic("implement me")
}

func TestFailures(t *testing.T) {
	//t.SkipNow()
	costVisitor := &CostVisitor{
		Costs:         cmap.New[*TerminatorCosts](),
		CircuitCost:   2,
		FailureCost:   50,
		SuccessCredit: 2,
	}

	terminator := mockTerminator{}

	var lock sync.Mutex

	workerCount := 10

	balances := make(chan int32, workerCount)

	for range workerCount {
		go func() {
			dial := 0
			done := 0
			fail := 0

			dialPct := 60
			donePct := 35

			var localBalance int32

			for i := range 1000000000 {
				next := rand.Intn(100)
				// successful dial
				if next < dialPct {
					if localBalance < 4000 {
						evt := xt.NewDialSucceeded(terminator)
						costVisitor.VisitDialSucceeded(evt)
						localBalance++
						dial++
					}
				} else if next < dialPct+donePct {
					if localBalance > 0 {
						evt := xt.NewCircuitRemoved(terminator)
						costVisitor.VisitCircuitRemoved(evt)
						localBalance--
						done++
					}
				} else {
					if rand.Intn(10) == 0 {
						evt := xt.NewDialFailedEvent(terminator)
						costVisitor.VisitDialFailed(evt)
						fail++
					}
				}
				if i%10 == 0 {
					costVisitor.CreditAll(2)
				}

				if i%1000000 == 0 {
					balances <- localBalance

					lock.Lock()
					balance := int32(0)
					first := false
					select {
					case balance = <-balances:
						first = true
					default:
					}

					if first {
						for range workerCount - 1 {
							balance += <-balances
						}

						cost := xt.GlobalCosts().GetDynamicCost("test")
						failureCost := int(costVisitor.GetFailureCost("test"))
						circuitCount := int(costVisitor.GetCircuitCount("test"))
						cachedCost := int(costVisitor.GetCost("test"))
						expected := 2*int(balance) + failureCost
						fmt.Printf("dial: %d, done: %d, fail: %d, balance %d, cost: %d, expected: %d, delta: %d, failureCost: %d, circuitCount: %d, cachedCost: %d\n",
							dial, done, fail, balance, cost, expected, expected-int(cost), failureCost, circuitCount, cachedCost)
						if int(cost) > expected {
							panic("bad state")
						}
					}
					lock.Unlock()

					if dialPct == 65 {
						dialPct = 20
						donePct = 79
					} else {
						dialPct = 65
						donePct = 30
					}
				}
			}
		}()
	}

	time.Sleep(10 * time.Second)
}
