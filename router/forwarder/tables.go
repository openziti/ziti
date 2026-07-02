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

package forwarder

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openziti/sdk-golang/v2/xgress"
	"github.com/openziti/ziti/v2/router/env"
	cmap "github.com/orcaman/concurrent-map/v2"
)

// circuitTable implements a directory of forwardTables, keyed by circuitId.
type circuitTable struct {
	circuits cmap.ConcurrentMap[string, *forwardTable]
}

func newCircuitTable() *circuitTable {
	return &circuitTable{
		circuits: cmap.New[*forwardTable](),
	}
}

func (st *circuitTable) setForwardTable(circuitId string, ft *forwardTable) {
	atomic.StoreInt64(&ft.last, time.Now().UnixMilli())
	st.circuits.Set(circuitId, ft)
}

func (st *circuitTable) getForwardTable(circuitId string, markActive bool) (*forwardTable, bool) {
	if ft, found := st.circuits.Get(circuitId); found {
		if markActive {
			atomic.StoreInt64(&ft.last, time.Now().UnixMilli())
		}
		return ft, true
	}
	return nil, false
}

func (st *circuitTable) removeForwardTable(circuitId string) {
	st.circuits.Remove(circuitId)
}

func (st *circuitTable) debug() string {
	out := fmt.Sprintf("circuits (%d):\n", st.circuits.Count())
	for i := range st.circuits.IterBuffered() {
		out += "\n"
		out += fmt.Sprintf("\tc/%s", i.Key)
		out += i.Val.debug()
	}
	return out
}

// forwardTable implements a directory of destinations, keyed by source address.
type forwardTable struct {
	ctrlId       string
	last         int64
	destinations cmap.ConcurrentMap[string, string]
}

func newForwardTable(ctrlId string) *forwardTable {
	return &forwardTable{
		ctrlId:       ctrlId,
		destinations: cmap.New[string](),
	}
}

func (ft *forwardTable) setForwardAddress(src, dst xgress.Address) {
	ft.destinations.Set(string(src), string(dst))
}

func (ft *forwardTable) getForwardAddress(src xgress.Address) (xgress.Address, bool) {
	if dst, found := ft.destinations.Get(string(src)); found {
		return xgress.Address(dst), true
	}
	return "", false
}

func (ft *forwardTable) debug() string {
	out := ""
	for i := range ft.destinations.IterBuffered() {
		out += fmt.Sprintf("\t\t@/%s -> @/%s\n", i.Key, i.Val)
	}
	return out
}

// destinationTable implements a directory of destinations, keyed by Address.
type destinationTable struct {
	destinations cmap.ConcurrentMap[string, env.Destination]

	// circuitLock guards the read-modify-write updates of xgress, which cmap doesn't make atomic. An
	// address lost from a circuit's list strands its destination, since teardown finds destinations there.
	circuitLock sync.Mutex
	xgress      cmap.ConcurrentMap[string, []xgress.Address]
}

func newDestinationTable() *destinationTable {
	return &destinationTable{
		destinations: cmap.New[env.Destination](),
		xgress:       cmap.New[[]xgress.Address](),
	}
}

func (dt *destinationTable) addDestination(addr xgress.Address, destination env.Destination) {
	dt.destinations.Set(string(addr), destination)
}

func (dt *destinationTable) getDestination(addr xgress.Address) (env.Destination, bool) {
	if dst, found := dt.destinations.Get(string(addr)); found {
		return dst, true
	}
	return nil, false
}

// takeDestination removes the destination at addr and returns it. Of several concurrent callers for
// the same addr, at most one gets found true.
func (dt *destinationTable) takeDestination(addr xgress.Address) (env.Destination, bool) {
	return dt.destinations.Pop(string(addr))
}

// takeDestinationIfMatches removes the destination at addr if it is destination, reporting whether it
// did. The compare and remove are atomic.
func (dt *destinationTable) takeDestinationIfMatches(addr xgress.Address, destination env.Destination) bool {
	return dt.destinations.RemoveCb(string(addr), func(_ string, v env.Destination, exists bool) bool {
		return exists && v == destination
	})
}

func (dt *destinationTable) removeDestinationIfMatches(addr xgress.Address, destination env.Destination) {
	dt.destinations.RemoveCb(string(addr), func(key string, v env.Destination, exists bool) bool {
		return exists && destination == v
	})
}

func (dt *destinationTable) linkDestinationToCircuit(circuitId string, address xgress.Address) {
	dt.circuitLock.Lock()
	defer dt.circuitLock.Unlock()

	addresses, _ := dt.xgress.Get(circuitId)
	dt.xgress.Set(circuitId, append(addresses, address))
}

func (dt *destinationTable) getAddressesForCircuit(circuitId string) ([]xgress.Address, bool) {
	if addresses, found := dt.xgress.Get(circuitId); found {
		return addresses, found
	}
	return nil, false
}

// takeAddressesForCircuit removes the circuit's address list and returns it in one step. An address
// linked afterwards starts a new list, which a later teardown of the circuit finds.
func (dt *destinationTable) takeAddressesForCircuit(circuitId string) ([]xgress.Address, bool) {
	dt.circuitLock.Lock()
	defer dt.circuitLock.Unlock()

	addresses, found := dt.xgress.Get(circuitId)
	if !found {
		return nil, false
	}
	dt.xgress.Remove(circuitId)
	return addresses, true
}

// unlinkDestinationFromCircuit removes address from the circuit's address list, leaving the circuit's
// other addresses linked, and removes the list once it is empty.
func (dt *destinationTable) unlinkDestinationFromCircuit(circuitId string, address xgress.Address) {
	dt.circuitLock.Lock()
	defer dt.circuitLock.Unlock()

	addresses, found := dt.xgress.Get(circuitId)
	if !found {
		return
	}
	var remaining []xgress.Address
	for _, addr := range addresses {
		if addr != address {
			remaining = append(remaining, addr)
		}
	}
	if len(remaining) == 0 {
		dt.xgress.Remove(circuitId)
		return
	}
	dt.xgress.Set(circuitId, remaining)
}

func (dt *destinationTable) debug() string {
	out := fmt.Sprintf("\ndestinations (%d):\n\n", dt.destinations.Count())
	for i := range dt.destinations.IterBuffered() {
		out += fmt.Sprintf("\t@/%s -> (%s (%p))\n", i.Key, reflect.TypeOf(i.Val).String(), i.Val)
	}
	out += "\n"

	out += fmt.Sprintf("xgress (%d):\n\n", dt.xgress.Count())
	for tuple := range dt.xgress.IterBuffered() {
		out += fmt.Sprintf("\tc/%s:\n", tuple.Key)
		addresses := tuple.Val
		for _, address := range addresses {
			out += fmt.Sprintf("\t\t@/%s\n", address)
		}
	}
	out += "\n"
	return out
}
