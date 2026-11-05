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

// StripedIdLocker is a fixed-size set of mutexes selected by hashing a string
// id. Operations on the same id always serialize; operations on different ids
// proceed concurrently unless they happen to hash to the same slot. Ids that
// collide on a slot share a lock (false contention), so size the locker
// comfortably above the expected concurrency to keep collisions rare.
//
// It is intended as a lighter-weight alternative to a single coarse mutex when
// the protected work is keyed by id and most concurrent callers touch
// different ids.
type StripedIdLocker struct {
	locks []sync.Mutex
}

// NewStripedIdLocker returns a StripedIdLocker with the given number of slots.
// slots is clamped to a minimum of 1.
func NewStripedIdLocker(slots int) *StripedIdLocker {
	if slots < 1 {
		slots = 1
	}
	return &StripedIdLocker{locks: make([]sync.Mutex, slots)}
}

// LockFor locks the slot for id and returns a function that unlocks it. The
// returned function must be called exactly once. Typical use:
//
//	defer locker.LockFor(id)()
func (self *StripedIdLocker) LockFor(id string) func() {
	m := &self.locks[self.indexFor(id)]
	m.Lock()
	return m.Unlock
}

// indexFor maps id to a slot using FNV-1a (inlined to avoid allocating a hasher
// on this hot path).
func (self *StripedIdLocker) indexFor(id string) uint32 {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	for i := 0; i < len(id); i++ {
		h ^= uint32(id[i])
		h *= prime32
	}
	return h % uint32(len(self.locks))
}
