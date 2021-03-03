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

package forwarder

import (
	"fmt"
	"github.com/openziti/fabric/router/xgress"
	"github.com/orcaman/concurrent-map"
	"reflect"
	"time"
)

// sessionTable implements a directory of forwardTables, keyed by sessionId.
//
type sessionTable struct {
	sessions cmap.ConcurrentMap // map[string]*forwardTable
}

type sessionForwardTable struct {
	last time.Time
	ft   *forwardTable
}

func newSessionTable() *sessionTable {
	return &sessionTable{
		sessions: cmap.New(),
	}
}

func (st *sessionTable) setForwardTable(sessionId string, ft *forwardTable) {
	st.sessions.Set(sessionId, ft)
}

func (st *sessionTable) getForwardTable(sessionId string) (*forwardTable, bool) {
	if ft, found := st.sessions.Get(sessionId); found {
		return ft.(*forwardTable), true
	}
	return nil, false
}

func (st *sessionTable) removeForwardTable(sessionId string) {
	st.sessions.Remove(sessionId)
}

func (st *sessionTable) debug() string {
	out := fmt.Sprintf("sessions (%d):\n", st.sessions.Count())
	for i := range st.sessions.IterBuffered() {
		out += "\n"
		out += fmt.Sprintf("\ts/%s", i.Key)
		out += i.Val.(*forwardTable).debug()
	}
	return out
}

// forwardTable implements a directory of destinations, keyed by source address.
//
type forwardTable struct {
	destinations cmap.ConcurrentMap // map[string]string
}

func newForwardTable() *forwardTable {
	return &forwardTable{
		destinations: cmap.New(),
	}
}

func (ft *forwardTable) setForwardAddress(src, dst xgress.Address) {
	ft.destinations.Set(string(src), string(dst))
}

func (ft *forwardTable) getForwardAddress(src xgress.Address) (xgress.Address, bool) {
	if dst, found := ft.destinations.Get(string(src)); found {
		return xgress.Address(dst.(string)), true
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
//
type destinationTable struct {
	destinations cmap.ConcurrentMap // map[xgress.Address]Destination
	xgress       cmap.ConcurrentMap // map[sessionId][]xgress.Address
}

func newDestinationTable() *destinationTable {
	return &destinationTable{
		destinations: cmap.New(),
		xgress:       cmap.New(),
	}
}

func (dt *destinationTable) addDestination(addr xgress.Address, destination Destination) {
	dt.destinations.Set(string(addr), destination)
}

func (dt *destinationTable) getDestination(addr xgress.Address) (Destination, bool) {
	if dst, found := dt.destinations.Get(string(addr)); found {
		return dst.(Destination), true
	}
	return nil, false
}

func (dt *destinationTable) removeDestination(addr xgress.Address) {
	dt.destinations.Remove(string(addr))
}

func (dt *destinationTable) linkDestinationToSession(sessionId string, address xgress.Address) {
	var addresses []xgress.Address
	if i, found := dt.xgress.Get(sessionId); found {
		addresses = i.([]xgress.Address)
	} else {
		addresses = make([]xgress.Address, 0)
	}
	addresses = append(addresses, address)
	dt.xgress.Set(sessionId, addresses)
}

func (dt *destinationTable) getAddressesForSession(sessionId string) ([]xgress.Address, bool) {
	if addresses, found := dt.xgress.Get(sessionId); found {
		return addresses.([]xgress.Address), found
	}
	return nil, false
}

func (dt *destinationTable) unlinkSession(sessionId string) {
	dt.xgress.Remove(sessionId)
}

func (dt *destinationTable) debug() string {
	out := fmt.Sprintf("\ndestinations (%d):\n\n", dt.destinations.Count())
	for i := range dt.destinations.IterBuffered() {
		out += fmt.Sprintf("\t@/%s -> (%s (%p))\n", i.Key, reflect.TypeOf(i.Val.(Destination)).String(), i.Val)
	}
	out += "\n"

	out += fmt.Sprintf("xgress (%d):\n\n", dt.xgress.Count())
	for i := range dt.xgress.IterBuffered() {
		out += fmt.Sprintf("\ts/%s:\n", i.Key)
		addresses := i.Val.([]xgress.Address)
		for _, address := range addresses {
			out += fmt.Sprintf("\t\t@/%s\n", address)
		}
	}
	out += "\n"
	return out
}
