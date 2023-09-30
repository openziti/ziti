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

package mesh

import (
	"sync"
	"time"
)

func newDeadline() *deadline {
	return &deadline{
		C: make(chan struct{}, 1),
	}
}

type deadline struct {
	C     chan struct{}
	timer *time.Timer
	lock  sync.Mutex
}

func (self *deadline) Trigger() {
	select {
	case self.C <- struct{}{}:
	default:
	}
}

func (self *deadline) SetTimeout(t time.Duration) {
	self.lock.Lock()
	defer self.lock.Unlock()

	if self.timer != nil {
		self.timer.Stop()
		self.timer = nil
	}

	select {
	case <-self.C:
	default:
	}

	self.timer = time.AfterFunc(t, self.Trigger)
}
