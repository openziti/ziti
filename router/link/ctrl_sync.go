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
)

// ctrlSynchronizer tracks what the router still owes each controller. A controller it has (re)connected to owes a
// full link refresh, and until that refresh is written the controller is not synced: incremental link
// reports to it are held, since a report that lands ahead of the refresh is either pruned by it or re-adds
// a link the refresh omitted. Safe for concurrent use.
type ctrlSynchronizer struct {
	lock   sync.Mutex
	states map[string]*ctrlSyncState
}

type ctrlSyncState struct {
	refreshPending bool
	// refreshInFlight is true while a pool task is sending this controller's refresh, so the loop queues
	// at most one at a time.
	refreshInFlight bool
	// reconnectGen counts reconnects. A refresh is built for the generation current when it was begun and
	// only satisfies the debt if no reconnect happened before it was written.
	reconnectGen uint64
}

func newCtrlSynchronizer() *ctrlSynchronizer {
	return &ctrlSynchronizer{states: map[string]*ctrlSyncState{}}
}

func (self *ctrlSynchronizer) state(ctrlId string) *ctrlSyncState {
	result, found := self.states[ctrlId]
	if !found {
		result = &ctrlSyncState{}
		self.states[ctrlId] = result
	}
	return result
}

// markReconnected records that ctrlId owes a full refresh.
func (self *ctrlSynchronizer) markReconnected(ctrlId string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	state := self.state(ctrlId)
	state.refreshPending = true
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

// forget drops tracking for controllers that are no longer in known and are owed nothing. A state that owes a
// refresh is kept even when its controller is absent from known: known is a snapshot, and
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
		settled := !state.refreshPending && !state.refreshInFlight
		if settled {
			delete(self.states, ctrlId)
		}
	}
}
