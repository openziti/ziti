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

package xlink_transwarp

import (
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

func (self *transmitBuffer) ack(gaps int) {
	self.cond.L.Lock()
	if gaps > 0 {
		if self.windowSize > 10 {
			self.windowSize /= 2
		}
	}
	self.windowContains = 0
	self.cond.L.Unlock()
}

func (self *transmitBuffer) accept(m *message) {
	self.cond.L.Lock()
	for self.windowContains >= self.windowSize {
		self.cond.Wait()
	}
	self.highWater = m.sequence
	self.windowContains++
	self.cond.L.Unlock()
}

func newTransmitBuffer() *transmitBuffer {
	txb := &transmitBuffer{
		windowSize: startingWindowSize,
	}
	txb.lock = new(sync.Mutex)
	txb.cond = sync.NewCond(txb.lock)
	return txb
}

type transmitBuffer struct {
	windowContains int
	windowSize     int
	highWater      int32
	lock           *sync.Mutex
	cond           *sync.Cond
}

func (self *receiveBuffer) receive(m *message) {
	self.lock.Lock()
	if m.sequence != self.highWater+1 {
		self.gaps++
	}
	if m.sequence < self.lowWater {
		self.lowWater = m.sequence
	}
	if m.sequence > self.highWater {
		self.highWater = m.sequence
	}
	self.lock.Unlock()
}

func (self *receiveBuffer) acker() {
	for {
		time.Sleep(1 * time.Second)

		self.lock.Lock()
		if time.Since(self.lastReport).Milliseconds() > 1000 {
			// send report
			logrus.Infof("sending transwarp window report")
			self.lastReport = time.Now()
		}
	}
}

func newReceiveBuffer(xlinkImpl *impl) *receiveBuffer {
	rxb := &receiveBuffer{
		lastReport: time.Now(),
		windowSize: startingWindowSize,
		xlinkImpl:  xlinkImpl,
	}
	rxb.lock = new(sync.Mutex)
	return rxb
}

type receiveBuffer struct {
	lowWater   int32
	highWater  int32
	count      int32
	gaps       int32
	windowSize int
	lastReport time.Time
	xlinkImpl  *impl
	lock       *sync.Mutex
}

const startingWindowSize = 10
