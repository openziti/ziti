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

package config

import (
	"github.com/openziti/foundation/v2/concurrenz"
	"sync"
)

type Listener[T any] interface {
	NotifyChanged(init bool, old T, new T)
}

type ListenerFunc[T any] func(init bool, old T, new T)

func (f ListenerFunc[T]) NotifyChanged(init bool, old T, new T) {
	f(init, old, new)
}

func NewConfigValue[T comparable]() *Value[T] {
	return &Value[T]{
		notifyInitialized: make(chan struct{}),
	}
}

type Value[T comparable] struct {
	lock              sync.Mutex
	initialized       bool
	notifyInitialized chan struct{}
	value             concurrenz.AtomicValue[T]
	listeners         concurrenz.CopyOnWriteSlice[Listener[T]]
}

func (self *Value[T]) Store(value T) {
	self.lock.Lock()
	defer self.lock.Unlock()

	first := !self.initialized
	old := self.value.Swap(value)

	if first || old != value {
		for _, l := range self.listeners.Value() {
			l.NotifyChanged(first, old, value)
		}
	}

	if first {
		self.initialized = true
		close(self.notifyInitialized)
	}
}

func (self *Value[T]) Load() T {
	return self.value.Load()
}

func (self *Value[T]) AddListener(listener Listener[T]) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.listeners.Append(listener)

	if self.initialized {
		listener.NotifyChanged(true, self.Load(), self.Load())
	}
}

func (self *Value[T]) RemoveListener(listener Listener[T]) {
	self.lock.Lock()
	defer self.lock.Unlock()

	self.listeners.Delete(listener)
}

func (self *Value[T]) GetInitNotifyChannel() <-chan struct{} {
	return self.notifyInitialized
}
