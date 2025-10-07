//go:build perftests

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

package common

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/openziti/foundation/v2/debugz"
	cmap "github.com/orcaman/concurrent-map/v2"
)

func (rdm *RouterDataModel) waitForQueueEmpty() {
	evt := &waitForQueueEmptyEvent{
		c: make(chan struct{}),
	}
	rdm.queueEvent(evt)
	<-evt.c
}

// waitForQueueEmptyEvent is a special event used for testing and synchronization. It signals
// when all pending identity updates have been processed by closing its channel.
type waitForQueueEmptyEvent struct {
	c chan struct{}
}

func (self *waitForQueueEmptyEvent) process(rdm *RouterDataModel) {
	if rdm.updatedIdentities.Count() == 0 {
		close(self.c)
	} else {
		time.Sleep(time.Second * 10)
		go func() {
			rdm.events <- self
		}()
	}
}

func newRandomStream[T HasId](m cmap.ConcurrentMap[string, T]) *randomStream[T] {
	result := &randomStream[T]{
		m:       m,
		c:       make(chan T, 10),
		closeCh: make(chan struct{}),
	}
	return result
}

type HasId interface {
	GetId() string
	comparable
}

type randomStream[T HasId] struct {
	m       cmap.ConcurrentMap[string, T]
	c       chan T
	stopped atomic.Bool
	closeCh chan struct{}
	in      <-chan cmap.Tuple[string, T]
}

func (self *randomStream[T]) run() {
	for !self.stopped.Load() {
		skip := rand.Intn(10)
		for i := 0; i < skip; i++ {
			self.getNextSeq()
		}
		val, ok := self.getNextSeq()
		if !ok {
			break
		}
		if reflect.ValueOf(val).IsNil() {
			panic("pushing nil value onto random stream")
		}
		select {
		case self.c <- val:
		case <-self.closeCh:
			return
		}
	}
}

func (self *randomStream[T]) getNextSeq() (T, bool) {
	var defaultT T

	if self.in == nil {
		if self.m.Count() == 0 {
			self.stop()
			return defaultT, false
		}
		self.in = self.m.IterBuffered()
	}

	select {
	case <-self.closeCh:
		return defaultT, false
	case t, ok := <-self.in:
		if !ok {
			self.in = nil
			return self.getNextSeq()
		}
		if reflect.ValueOf(t.Val).IsNil() {
			fmt.Println(reflect.TypeOf(self.m).String())
			panic("get nil value from map iterator")
		}
		return t.Val, true
	}
}

func (self *randomStream[T]) stop() {
	debugz.DumpLocalStack()
	if self.stopped.CompareAndSwap(false, true) {
		close(self.closeCh)
	}
}

func (self *randomStream[T]) Next() T {
	for {
		select {
		case <-self.closeCh:
			var defaultT T
			return defaultT
		case v := <-self.c:
			if self.m.Has(v.GetId()) {
				return v
			}
		}
	}
}

func (self *randomStream[T]) GetFilteredSet(n int, filterF func(T) bool, mapF func(T) string) map[string]struct{} {
	result := map[string]struct{}{}
	for len(result) < n {
		next := self.Next()
		if filterF(next) {
			result[mapF(next)] = struct{}{}
		}
	}
	return result
}
