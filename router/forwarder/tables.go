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
	"github.com/openziti/fabric/router/xgress"
	"github.com/orcaman/concurrent-map/v2"
	"reflect"
	"sync/atomic"
	"time"
)

// circuitTable implements a directory of forwardTables, keyed by circuitId.
type circuitTable struct {
	circuits cmap.ConcurrentMap[*forwardTable]
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

func (st *circuitTable) getForwardTable(circuitId string) (*forwardTable, bool) {
	if ft, found := st.circuits.Get(circuitId); found {
		atomic.StoreInt64(&ft.last, time.Now().UnixMilli())
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
	destinations cmap.ConcurrentMap[string]
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
	destinations cmap.ConcurrentMap[Destination]
	xgress       cmap.ConcurrentMap[[]xgress.Address]
}

func newDestinationTable() *destinationTable {
	return &destinationTable{
		destinations: cmap.New[Destination](),
		xgress:       cmap.New[[]xgress.Address](),
	}
}

func (dt *destinationTable) addDestination(addr xgress.Address, destination Destination) {
	dt.destinations.Set(string(addr), destination)
}

func (dt *destinationTable) addDestinationIfAbsent(addr xgress.Address, destination Destination) bool {
	return dt.destinations.SetIfAbsent(string(addr), destination)
}

func (dt *destinationTable) getDestination(addr xgress.Address) (Destination, bool) {
	if dst, found := dt.destinations.Get(string(addr)); found {
		return dst, true
	}
	return nil, false
}

func (dt *destinationTable) removeDestination(addr xgress.Address) {
	dt.destinations.Remove(string(addr))
}

func (dt *destinationTable) linkDestinationToCircuit(circuitId string, address xgress.Address) {
	var addresses []xgress.Address
	if i, found := dt.xgress.Get(circuitId); found {
		addresses = i
	} else {
		addresses = make([]xgress.Address, 0)
	}
	addresses = append(addresses, address)
	dt.xgress.Set(circuitId, addresses)
}

func (dt *destinationTable) getAddressesForCircuit(circuitId string) ([]xgress.Address, bool) {
	if addresses, found := dt.xgress.Get(circuitId); found {
		return addresses, found
	}
	return nil, false
}

func (dt *destinationTable) unlinkCircuit(circuitId string) {
	dt.xgress.Remove(circuitId)
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
