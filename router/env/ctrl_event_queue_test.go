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

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestCtrlEventQueue_DeliversInOrderToSlowListener: a listener that is slow on one event must still see
// the events after it in the order they were queued, and queuing must not wait for it.
func TestCtrlEventQueue_DeliversInOrderToSlowListener(t *testing.T) {
	var lock sync.Mutex
	var seen []CtrlEventType
	release := make(chan struct{})
	first := true

	queue := newCtrlEventQueue(CtrlEventListenerFunc(func(event CtrlEvent) {
		if first {
			first = false
			<-release
		}
		lock.Lock()
		seen = append(seen, event.Type)
		lock.Unlock()
	}))

	expected := []CtrlEventType{ControllerAdded, ControllerDisconnected, ControllerReconnected, ControllerLeaderChange}
	done := make(chan struct{})
	go func() {
		for _, eventType := range expected {
			queue.enqueue(CtrlEvent{Type: eventType})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("enqueue must not block on a slow listener")
	}

	lock.Lock()
	require.Empty(t, seen, "nothing is delivered while the listener is still handling the first event")
	lock.Unlock()

	close(release)
	require.Eventually(t, func() bool {
		lock.Lock()
		defer lock.Unlock()
		return len(seen) == len(expected)
	}, time.Second, time.Millisecond)

	lock.Lock()
	defer lock.Unlock()
	require.Equal(t, expected, seen)
}

// TestCtrlEventQueue_ResumesAfterDraining: once a drain finishes, a later event must start a new one
// rather than sit in the queue.
func TestCtrlEventQueue_ResumesAfterDraining(t *testing.T) {
	received := make(chan CtrlEventType, 4)
	queue := newCtrlEventQueue(CtrlEventListenerFunc(func(event CtrlEvent) {
		received <- event.Type
	}))

	queue.enqueue(CtrlEvent{Type: ControllerAdded})
	require.Equal(t, ControllerAdded, <-received)

	require.Eventually(t, func() bool {
		queue.lock.Lock()
		defer queue.lock.Unlock()
		return !queue.draining
	}, time.Second, time.Millisecond)

	queue.enqueue(CtrlEvent{Type: ControllerDisconnected})
	select {
	case eventType := <-received:
		require.Equal(t, ControllerDisconnected, eventType)
	case <-time.After(time.Second):
		t.Fatal("event queued after a drain finished was never delivered")
	}
}
