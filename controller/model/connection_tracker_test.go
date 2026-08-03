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

package model

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"

	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/controller/event"
)

// fakeCtrlChannel implements the part of ctrlchan.CtrlChannel that the connection tracker
// uses. The embedded interface is left nil so that any other method panics rather than
// quietly returning a zero value.
type fakeCtrlChannel struct {
	ctrlchan.CtrlChannel
	id     string
	closed atomic.Bool
}

func (self *fakeCtrlChannel) PeerId() string {
	return self.id
}

func (self *fakeCtrlChannel) IsClosed() bool {
	return self.closed.Load()
}

func newTestConnectionTracker(t *testing.T) *ConnectionTracker {
	closeNotify := make(chan struct{})
	t.Cleanup(func() {
		close(closeNotify)
	})

	return &ConnectionTracker{
		connections:     cmap.New[*identityConnections](),
		eventDispatcher: event.DispatcherMock{},
		scanInterval:    time.Millisecond,
		unknownTimeout:  time.Minute,
		closeNotify:     closeNotify,
	}
}

// TestConnectionTrackerLockOrdering ensures that the scan loop and connect/disconnect
// handling can run concurrently against the same identities without deadlocking.
//
// The tracker uses two locks, the cmap shard lock and the per-identity lock. Acquiring
// them in opposite orders in different code paths deadlocks permanently, and because the
// shard lock is then never released, every identity read in the controller blocks behind
// it.
func TestConnectionTrackerLockOrdering(t *testing.T) {
	tracker := newTestConnectionTracker(t)

	// A small identity set keeps the scan loop and the connect/disconnect handling
	// contending on the same entries and the same cmap shards.
	identityIds := []string{"identity-1", "identity-2", "identity-3", "identity-4"}

	var stop atomic.Bool
	var wg sync.WaitGroup

	for _, identityId := range identityIds {
		wg.Add(1)
		go func(identityId string) {
			defer wg.Done()
			ch := &fakeCtrlChannel{id: "router-1"}
			for !stop.Load() {
				// leaves the entry with no routers, making it a candidate for the scan
				// loop to reap while this goroutine is still touching it
				tracker.MarkConnected(identityId, ch)
				tracker.MarkDisconnected(identityId, ch)
			}
		}(identityId)
	}

	// An identity no other goroutine touches, so that the scan loop is the only thing
	// that could take it offline. Nothing may drop an entry that has a live router
	// connection, so if the scan loop removes it based on a view taken before the
	// reconnect, this sees it.
	var sawUnexpectedState atomic.Bool
	wg.Add(1)
	go func() {
		defer wg.Done()
		ch := &fakeCtrlChannel{id: "router-2"}
		for !stop.Load() {
			tracker.MarkConnected("identity-reconnecting", ch)
			if tracker.GetIdentityOnlineState("identity-reconnecting") != IdentityStateOnline {
				sawUnexpectedState.Store(true)
				return
			}
			tracker.MarkDisconnected("identity-reconnecting", ch)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			tracker.ScanForDisconnectedRouters()
		}
	}()

	time.AfterFunc(2*time.Second, func() {
		stop.Store(true)
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		buf := make([]byte, 4*1024*1024)
		n := runtime.Stack(buf, true)
		t.Fatalf("connection tracker deadlocked, goroutine dump follows:\n%s", buf[:n])
	}

	require.False(t, sawUnexpectedState.Load(),
		"identity reported as not online while it had a live router connection")
}

// TestConnectionTrackerReapsDisconnectedIdentities covers the entry removal path that the
// scan loop uses, since that is where the lock ordering has to be respected.
func TestConnectionTrackerReapsDisconnectedIdentities(t *testing.T) {
	req := require.New(t)
	tracker := newTestConnectionTracker(t)

	ch := &fakeCtrlChannel{id: "router-1"}

	tracker.MarkConnected("identity-1", ch)
	req.Equal(IdentityStateOnline, tracker.GetIdentityOnlineState("identity-1"))

	tracker.ScanForDisconnectedRouters()
	req.Equal(1, tracker.connections.Count(), "identity with a connected router should not be reaped")

	tracker.MarkDisconnected("identity-1", ch)
	req.Equal(IdentityStateOffline, tracker.GetIdentityOnlineState("identity-1"))

	tracker.ScanForDisconnectedRouters()
	req.Equal(0, tracker.connections.Count(), "identity with no connected routers should be reaped")
}

// TestConnectionTrackerRemoveIfEmptyRechecksMapValue covers the window between the scan
// loop deciding an entry is empty and the entry actually being removed. Nothing is locked
// across that window, so an identity can reconnect inside it and the removal has to
// notice.
//
// The scan loop is driven a step at a time here because that window cannot be held open
// from outside; removeIfEmpty is exactly the step that runs after the decision.
func TestConnectionTrackerRemoveIfEmptyRechecksMapValue(t *testing.T) {
	req := require.New(t)
	tracker := newTestConnectionTracker(t)

	ch := &fakeCtrlChannel{id: "router-1"}

	tracker.MarkConnected("identity-1", ch)
	tracker.MarkDisconnected("identity-1", ch)

	// the state the scan loop observes, and acts on, before removing the entry
	entry, found := tracker.connections.Get("identity-1")
	req.True(found)
	entry.RLock()
	empty := len(entry.routers) == 0
	entry.RUnlock()
	req.True(empty, "scan loop would decide to remove this entry")

	// the identity reconnects before the removal runs
	tracker.MarkConnected("identity-1", ch)

	tracker.removeIfEmpty("identity-1")

	req.Equal(1, tracker.connections.Count(), "reconnected identity should not be removed")
	req.Equal(IdentityStateOnline, tracker.GetIdentityOnlineState("identity-1"))
}

// TestConnectionTrackerRemoveIfEmptyRemovesEmptyEntry is the other half of
// TestConnectionTrackerRemoveIfEmptyRechecksMapValue, so that the recheck cannot be
// satisfied by simply never removing anything.
func TestConnectionTrackerRemoveIfEmptyRemovesEmptyEntry(t *testing.T) {
	req := require.New(t)
	tracker := newTestConnectionTracker(t)

	ch := &fakeCtrlChannel{id: "router-1"}

	tracker.MarkConnected("identity-1", ch)
	tracker.MarkDisconnected("identity-1", ch)
	req.Equal(1, tracker.connections.Count())

	tracker.removeIfEmpty("identity-1")
	req.Equal(0, tracker.connections.Count())
}
