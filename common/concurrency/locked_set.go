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

package concurrency

import "sync"

// LockedSet is a set guarded by a single RWMutex. Safe for concurrent use.
//
// It is intended for sets small enough, or written rarely enough, that one lock is not a bottleneck.
// A sharded map is the alternative when writes are hot, at the cost of a map and a mutex per shard for
// every set that exists; a copy-on-write set is the alternative when reads dominate and the set is
// rarely written, at the cost of copying every entry per write.
type LockedSet[T comparable] struct {
	lock   sync.RWMutex
	values map[T]struct{}
}

// NewLockedSet returns an empty LockedSet.
func NewLockedSet[T comparable]() *LockedSet[T] {
	return &LockedSet[T]{values: map[T]struct{}{}}
}

// Add adds value to the set. Adding a value already present is a no-op.
func (self *LockedSet[T]) Add(value T) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.values[value] = struct{}{}
}

// Remove removes value from the set. Removing a value not present is a no-op.
func (self *LockedSet[T]) Remove(value T) {
	self.lock.Lock()
	defer self.lock.Unlock()
	delete(self.values, value)
}

// RemoveIf removes value only if it is present and shouldRemove reports that it should be.
//
// shouldRemove runs with the set's write lock held, so it must not call back into this set. Use it when
// the decision depends on state outside the set that could otherwise change between the check and the
// removal.
func (self *LockedSet[T]) RemoveIf(value T, shouldRemove func() bool) {
	self.lock.Lock()
	defer self.lock.Unlock()
	if _, found := self.values[value]; found && shouldRemove() {
		delete(self.values, value)
	}
}

// Contains reports whether value is in the set.
func (self *LockedSet[T]) Contains(value T) bool {
	self.lock.RLock()
	defer self.lock.RUnlock()
	_, found := self.values[value]
	return found
}

// Values returns a snapshot of the set's contents in unspecified order. The caller owns the slice, and it
// does not track later changes to the set.
func (self *LockedSet[T]) Values() []T {
	self.lock.RLock()
	defer self.lock.RUnlock()
	result := make([]T, 0, len(self.values))
	for value := range self.values {
		result = append(result, value)
	}
	return result
}
