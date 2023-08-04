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

package events

import (
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/event"
	"github.com/openziti/fabric/events"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/storage/boltz"
	"io"
)

func NewDispatcher(n *network.Network, dbProvider persistence.DbProvider, stores *persistence.Stores, closeNotify <-chan struct{}) *Dispatcher {
	result := &Dispatcher{
		network:          n,
		fabricDispatcher: n.GetEventDispatcher(),
		dbProvider:       dbProvider,
		stores:           stores,
		closeNotify:      closeNotify,
	}

	fabricDispatcher := n.GetEventDispatcher()

	fabricDispatcher.RegisterEventTypeFunctions(ApiSessionEventNS, result.registerApiSessionEventHandler, result.unregisterApiSessionEventHandler)
	fabricDispatcher.RegisterEventTypeFunctions(EntityCountEventNS, result.registerEntityCountEventHandler, result.unregisterEntityCountEventHandler)
	fabricDispatcher.RegisterEventTypeFunctions(SessionEventNS, result.registerSessionEventHandler, result.unregisterSessionEventHandler)

	fabricDispatcher.RegisterFormatterFactory("json", event.FormatterFactoryF(func(sink event.FormattedEventSink) io.Closer {
		return &EdgeJsonFormatter{
			JsonFormatter: *events.NewJsonFormatter(16, sink),
		}
	}))

	fabricDispatcher.RegisterEventHandlerFactory("file", edgeFileEventLoggerFactory{})
	fabricDispatcher.RegisterEventHandlerFactory("stdout", edgeStdOutLoggerFactory{})

	result.InitializeEvents()

	return result
}

type Dispatcher struct {
	apiSessionEventHandlers  concurrenz.CopyOnWriteSlice[ApiSessionEventHandler]
	entityCountEventHandlers concurrenz.CopyOnWriteSlice[*entityCountState]
	sessionEventHandlers     concurrenz.CopyOnWriteSlice[SessionEventHandler]

	network          *network.Network
	fabricDispatcher event.Dispatcher
	dbProvider       persistence.DbProvider
	stores           *persistence.Stores
	closeNotify      <-chan struct{}
}

func (self *Dispatcher) InitializeEvents() {
	self.initApiSessionEvents(self.stores)
	self.initSessionEvents(self.stores)
	self.initEntityEvents()

	fabricStores := map[boltz.Store]struct{}{}

	for _, store := range self.network.GetStores().GetStoreList() {
		fabricStores[store] = struct{}{}
	}

	for _, store := range self.stores.GetStores() {
		if _, found := fabricStores[store]; !found {
			self.fabricDispatcher.AddEntityChangeSource(store)
		}
	}
}
