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

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestStripedIdLocker_SameIdSerializes verifies that two lockers for the same id
// are mutually exclusive: the second LockFor blocks until the first unlocks.
func TestStripedIdLocker_SameIdSerializes(t *testing.T) {
	locker := NewStripedIdLocker(16)

	unlock := locker.LockFor("link-1")

	acquired := make(chan struct{})
	go func() {
		secondUnlock := locker.LockFor("link-1")
		close(acquired)
		secondUnlock()
	}()

	select {
	case <-acquired:
		t.Fatal("second LockFor for the same id acquired while the first was still held")
	case <-time.After(50 * time.Millisecond):
		// expected: still blocked
	}

	unlock()

	select {
	case <-acquired:
		// expected: unblocked once the first lock was released
	case <-time.After(time.Second):
		t.Fatal("second LockFor did not acquire after the first was released")
	}
}

// TestStripedIdLocker_MutualExclusion runs many goroutines incrementing per-id
// counters guarded only by the striped locker. With correct per-id
// serialization every counter ends at the expected total. Run with -race to
// also catch incorrect sharing.
func TestStripedIdLocker_MutualExclusion(t *testing.T) {
	const (
		ids           = 50
		goroutines    = 32
		incsPerWorker = 200
	)

	// Use more ids than slots so multiple ids share slots; correctness must not
	// depend on a 1:1 id-to-slot mapping.
	locker := NewStripedIdLocker(8)
	counters := make([]int, ids)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < incsPerWorker; i++ {
				id := fmt.Sprintf("id-%d", i%ids)
				unlock := locker.LockFor(id)
				counters[i%ids]++
				unlock()
			}
		}()
	}
	wg.Wait()

	expected := goroutines * incsPerWorker / ids
	for i, c := range counters {
		if c != expected {
			t.Fatalf("counter[%d] = %d, expected %d", i, c, expected)
		}
	}
}

// TestStripedIdLocker_DifferentIdsConcurrent verifies that ids landing on
// different slots do not block each other.
func TestStripedIdLocker_DifferentIdsConcurrent(t *testing.T) {
	locker := NewStripedIdLocker(256)

	// Find two ids that map to different slots.
	a := "router-a"
	var b string
	for i := 0; ; i++ {
		candidate := fmt.Sprintf("router-%d", i)
		if locker.indexFor(candidate) != locker.indexFor(a) {
			b = candidate
			break
		}
	}

	unlockA := locker.LockFor(a)
	defer unlockA()

	done := make(chan struct{})
	go func() {
		locker.LockFor(b)() // should not block on a's slot
		close(done)
	}()

	select {
	case <-done:
		// expected
	case <-time.After(time.Second):
		t.Fatal("LockFor on a different slot blocked while an unrelated slot was held")
	}
}
