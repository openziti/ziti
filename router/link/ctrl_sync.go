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

package link

import (
	"sync"

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/xlink"
)

// ctrlSynchronizer tracks what the router still owes each controller. A controller it has (re)connected to owes a
// full link refresh, and until that refresh is written the controller is not synced: incremental link
// reports to it are held, since a report that lands ahead of the refresh is either pruned by it or re-adds
// a link the refresh omitted. Each controller also owes the current link listener set whenever that set
// has changed since it was last written there. Safe for concurrent use.
type ctrlSynchronizer struct {
	lock sync.Mutex
	// listenersGen counts listener set changes. It starts at 1 so a controller that has never been sent
	// the set (sentGen 0) is pending.
	listenersGen uint64
	states       map[string]*ctrlSyncState
}

type ctrlSyncState struct {
	refreshPending bool
	// refreshInFlight is true while a pool task is sending this controller's refresh, so the loop queues
	// at most one at a time.
	refreshInFlight bool
	// reconnectGen counts reconnects. A refresh is built for the generation current when it was begun and
	// only satisfies the debt if no reconnect happened before it was written.
	reconnectGen     uint64
	listenersSentGen uint64
}

func newCtrlSynchronizer() *ctrlSynchronizer {
	return &ctrlSynchronizer{listenersGen: 1, states: map[string]*ctrlSyncState{}}
}

func (self *ctrlSynchronizer) state(ctrlId string) *ctrlSyncState {
	result, found := self.states[ctrlId]
	if !found {
		result = &ctrlSyncState{}
		self.states[ctrlId] = result
	}
	return result
}

// markReconnected records that ctrlId owes a full refresh and the current listener set.
func (self *ctrlSynchronizer) markReconnected(ctrlId string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	state := self.state(ctrlId)
	state.refreshPending = true
	state.listenersSentGen = 0
	state.reconnectGen++
}

// beginRefresh claims the refresh owed to ctrlId, returning the reconnect generation it is being built for.
// It reports false when no refresh is owed or one is already in flight.
func (self *ctrlSynchronizer) beginRefresh(ctrlId string) (gen uint64, ok bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	state, found := self.states[ctrlId]
	if !found || !state.refreshPending || state.refreshInFlight {
		return 0, false
	}
	state.refreshInFlight = true
	return state.reconnectGen, true
}

// markRefreshWritten records that the refresh begun for gen was written to ctrlId. It reports false, leaving
// the refresh owed, when a reconnect happened after gen was taken: that refresh described a channel state
// since replaced, so the controller is not synced by it.
func (self *ctrlSynchronizer) markRefreshWritten(ctrlId string, gen uint64) bool {
	self.lock.Lock()
	defer self.lock.Unlock()
	state := self.state(ctrlId)
	state.refreshInFlight = false
	if state.reconnectGen != gen {
		return false
	}
	state.refreshPending = false
	return true
}

// markRefreshFailed records that the refresh in flight for ctrlId was not written. The debt stays.
func (self *ctrlSynchronizer) markRefreshFailed(ctrlId string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.state(ctrlId).refreshInFlight = false
}

// isRefreshInFlight reports whether a refresh for ctrlId is being sent.
func (self *ctrlSynchronizer) isRefreshInFlight(ctrlId string) bool {
	self.lock.Lock()
	defer self.lock.Unlock()
	state, found := self.states[ctrlId]
	return found && state.refreshInFlight
}

// isSynced reports whether incremental link reports may be sent to ctrlId. A controller never seen by
// markReconnected is synced, which keeps the pre-reconnect-tracking behavior for anything not routed through it.
func (self *ctrlSynchronizer) isSynced(ctrlId string) bool {
	self.lock.Lock()
	defer self.lock.Unlock()
	state, found := self.states[ctrlId]
	return !found || !state.refreshPending
}

// markListenersChanged records that the listener set changed, making it pending for every controller. It
// returns the new generation.
func (self *ctrlSynchronizer) markListenersChanged() uint64 {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.listenersGen++
	return self.listenersGen
}

// currentListenersGen returns the generation a send built now should report with markListenersSent.
func (self *ctrlSynchronizer) currentListenersGen() uint64 {
	self.lock.Lock()
	defer self.lock.Unlock()
	return self.listenersGen
}

// isListenersSendPending reports whether ctrlId has not been sent the current listener set.
func (self *ctrlSynchronizer) isListenersSendPending(ctrlId string) bool {
	self.lock.Lock()
	defer self.lock.Unlock()
	state, found := self.states[ctrlId]
	return !found || state.listenersSentGen != self.listenersGen
}

// reconnectGen returns ctrlId's current reconnect generation, for fencing a send begun now against a later
// reconnect.
func (self *ctrlSynchronizer) reconnectGen(ctrlId string) uint64 {
	self.lock.Lock()
	defer self.lock.Unlock()
	return self.state(ctrlId).reconnectGen
}

// markListenersSent records that the listener set of generation gen was written to ctrlId over the connection
// of reconnect generation reconnectGen. A set that changed again since gen was built stays pending, and so
// does the whole set when ctrlId has reconnected since reconnectGen was taken: that write reached the previous
// connection, not the current one.
func (self *ctrlSynchronizer) markListenersSent(ctrlId string, gen uint64, reconnectGen uint64) {
	self.lock.Lock()
	defer self.lock.Unlock()
	state := self.state(ctrlId)
	if state.reconnectGen != reconnectGen {
		return
	}
	if gen > state.listenersSentGen {
		state.listenersSentGen = gen
	}
}

// forget drops tracking for controllers that are no longer in known and are owed nothing. A state that owes a
// refresh or the listener set is kept even when its controller is absent from known: known is a snapshot, and
// a controller that connected after it was taken has just recorded that debt. Dropping the state would read
// as synced and nothing would ever announce to it. A settled state is safe to drop, since a controller that
// reappears records a fresh debt.
func (self *ctrlSynchronizer) forget(known map[string]struct{}) {
	self.lock.Lock()
	defer self.lock.Unlock()
	for ctrlId, state := range self.states {
		if _, ok := known[ctrlId]; ok {
			continue
		}
		settled := !state.refreshPending && !state.refreshInFlight && state.listenersSentGen == self.listenersGen
		if settled {
			delete(self.states, ctrlId)
		}
	}
}

// ListenersToProto converts the router's link listeners to the wire shape used in the hello ListenersHeader
// and in UpdateLinkListeners, in the order given.
func ListenersToProto(listeners []xlink.Listener) *ctrl_pb.Listeners {
	result := &ctrl_pb.Listeners{}
	for _, listener := range listeners {
		result.Listeners = append(result.Listeners, &ctrl_pb.Listener{
			Address:      listener.GetAdvertisement(),
			Protocol:     listener.GetLinkProtocol(),
			Groups:       listener.GetGroups(),
			LocalBinding: listener.GetLocalBinding(),
		})
	}
	return result
}
