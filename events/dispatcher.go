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
	"github.com/openziti/foundation/v2/concurrenz"
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

	fabricDispatcher.RegisterEventType(ApiSessionEventNS, result.registerApiSessionEventHandler)
	fabricDispatcher.RegisterEventType(EntityCountEventNS, result.registerEntityCountEventHandler)
	fabricDispatcher.RegisterEventType(SessionEventNS, result.registerSessionEventHandler)

	fabricDispatcher.RegisterEventHandlerFactory("file", edgeFileEventLoggerFactory{})
	fabricDispatcher.RegisterEventHandlerFactory("stdout", edgeStdOutLoggerFactory{})

	result.InitializeEvents()

	return result
}

type Dispatcher struct {
	apiSessionEventHandlers concurrenz.CopyOnWriteSlice[ApiSessionEventHandler]
	sessionEventHandlers    concurrenz.CopyOnWriteSlice[SessionEventHandler]

	network          *network.Network
	fabricDispatcher event.Dispatcher
	dbProvider       persistence.DbProvider
	stores           *persistence.Stores
	closeNotify      <-chan struct{}
}

func (self *Dispatcher) InitializeEvents() {
	self.initApiSessionEvents(self.stores)
	self.initSessionEvents(self.stores)
}
