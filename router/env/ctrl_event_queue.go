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

package env

import "sync"

// ctrlEventQueue delivers ctrl events to one listener in the order they were queued, one at a time,
// on a goroutine of its own. A listener that keys on transitions must see a disconnect before the
// reconnect that follows it, and a slow listener must delay neither the caller nor other listeners.
type ctrlEventQueue struct {
	listener CtrlEventListener
	lock     sync.Mutex
	pending  []CtrlEvent
	draining bool
}

func newCtrlEventQueue(listener CtrlEventListener) *ctrlEventQueue {
	return &ctrlEventQueue{listener: listener}
}

// enqueue adds event and starts a drain if none is running. It never blocks on the listener.
func (self *ctrlEventQueue) enqueue(event CtrlEvent) {
	self.lock.Lock()
	self.pending = append(self.pending, event)
	startDrain := !self.draining
	self.draining = true
	self.lock.Unlock()

	if startDrain {
		go self.drain()
	}
}

func (self *ctrlEventQueue) drain() {
	for {
		self.lock.Lock()
		if len(self.pending) == 0 {
			self.draining = false
			self.lock.Unlock()
			return
		}
		event := self.pending[0]
		self.pending = self.pending[1:]
		self.lock.Unlock()

		self.listener.NotifyOfCtrlEvent(event)
	}
}
