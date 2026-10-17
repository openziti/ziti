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

package gossip

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewSkewMonitor_ThresholdTracksTheTombstoneLifetime(t *testing.T) {
	req := require.New(t)

	// A tenth of the lifetime, because that is what makes skew matter: a deadline one node stamps and
	// another reads only misleads in proportion to the lifetime it consumes.
	req.Equal(time.Minute, NewSkewMonitor(10*time.Minute).threshold)

	// The floor keeps a short lifetime from making ordinary transit a warning.
	req.Equal(skewWarnFloor, NewSkewMonitor(10*time.Second).threshold)

	// Nothing expires on a deadline, so nothing depends on a clock.
	req.Nil(NewSkewMonitor(0), "a store that expires nothing needs no clock comparison")
	req.Nil(NewSkewMonitor(-time.Second))
}

func TestSkewMonitor_ObserveIgnoresWhatItCannotJudge(t *testing.T) {
	m := NewSkewMonitor(10 * time.Minute)
	require.NotNil(t, m)

	// An unstamped message is not evidence the clocks agree, so it is not counted either way.
	m.Observe("peer1", 0)
	require.Empty(t, m.lastWarnFor, "an unstamped message must not be recorded as an observation")

	// Within threshold, in both directions.
	m.Observe("peer1", time.Now().Add(-30*time.Second).UnixNano())
	m.Observe("peer1", time.Now().Add(30*time.Second).UnixNano())
	require.Empty(t, m.lastWarnFor)

	// A nil monitor is usable, so a store that expires nothing needs no branch at the call site.
	var none *SkewMonitor
	require.NotPanics(t, func() { none.Observe("peer1", time.Now().UnixNano()) })
}

func TestSkewMonitor_ReportsBothDirectionsOncePerInterval(t *testing.T) {
	req := require.New(t)

	for _, sentAt := range map[string]time.Time{
		"peer behind us":   time.Now().Add(-5 * time.Minute),
		"peer ahead of us": time.Now().Add(5 * time.Minute),
	} {
		m := NewSkewMonitor(10 * time.Minute)
		m.Observe("peer1", sentAt.UnixNano())
		req.Len(m.lastWarnFor, 1, "skew past the threshold must be reported whichever way it runs")

		first := m.lastWarnFor["peer1"]
		m.Observe("peer1", sentAt.UnixNano())
		req.Equal(first, m.lastWarnFor["peer1"],
			"skew persists, so an unbounded report would be a line per exchange for as long as it lasts")

		// A different peer is judged on its own.
		m.Observe("peer2", sentAt.UnixNano())
		req.Len(m.lastWarnFor, 2)
	}
}

func TestSkewMonitor_ConcurrentObserve(t *testing.T) {
	m := NewSkewMonitor(10 * time.Minute)
	sentAt := time.Now().Add(-5 * time.Minute).UnixNano()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				m.Observe("peer1", sentAt)
			}
		}()
	}
	wg.Wait()

	require.Len(t, m.lastWarnFor, 1)
}

// TestSkewMonitor_ForgetsPeersItWillNeverSuppress covers the leak this map is one deletion away from: it is
// keyed by peer id, and a deleted router's id never returns, since a recreated router is assigned a new one.
// A fleet that recreates routers hands this an unbounded stream of ids. Entries older than the interval
// they enforce suppress nothing and must not be kept against ids nothing will report under again.
func TestSkewMonitor_ForgetsPeersItWillNeverSuppress(t *testing.T) {
	req := require.New(t)

	m := NewSkewMonitor(10 * time.Minute)
	skewed := time.Now().Add(-5 * time.Minute).UnixNano()

	// A fleet's worth of routers, each seen once and never again.
	for i := 0; i < 100; i++ {
		m.Observe(fmt.Sprintf("router-%d", i), skewed)
	}
	req.Len(m.lastWarnFor, 100)

	// Age every one of them past the point where it could suppress anything, and open the sweep gate,
	// which is what a real interval passing would do to both.
	m.lock.Lock()
	for peer := range m.lastWarnFor {
		m.lastWarnFor[peer] = time.Now().Add(-2 * skewReportInterval)
	}
	m.nextPrune = time.Now().Add(-time.Second)
	m.lock.Unlock()

	m.Observe("router-new", skewed)
	req.Len(m.lastWarnFor, 1, "ids that can no longer suppress a report must not be retained")

	remaining, seen := m.lastWarnFor["router-new"]
	req.True(seen, "the peer being reported must be the one kept")
	req.WithinDuration(time.Now(), remaining, time.Minute)
}

// TestSkewMonitor_PruningDoesNotBreakSuppression: pruning runs while adding, so a peer still inside its
// interval has to survive it, or the rate limit it exists to enforce would be lost.
func TestSkewMonitor_PruningDoesNotBreakSuppression(t *testing.T) {
	req := require.New(t)

	m := NewSkewMonitor(10 * time.Minute)
	skewed := time.Now().Add(-5 * time.Minute).UnixNano()

	m.Observe("peer1", skewed)
	first := m.lastWarnFor["peer1"]

	// A second peer reports, which is what triggers a prune.
	m.Observe("peer2", skewed)
	req.Len(m.lastWarnFor, 2)

	// peer1 is still inside its interval, so it is neither pruned nor reported again.
	m.Observe("peer1", skewed)
	req.Equal(first, m.lastWarnFor["peer1"], "a recently reported peer must still be suppressed")
	req.Len(m.lastWarnFor, 2)
}

// TestSkewMonitor_AFleetReportingAtOnceDoesNotSweepPerPeer covers the cost of the sweep, not its effect.
//
// A fleet sharing one clock offset is what an NTP failure looks like, so every router reporting at once is
// the case this exists to catch rather than a corner. Sweeping as each new peer is added would walk a map
// that every previous one just grew, under a single lock, on the goroutines reading each router's control
// channel. Nothing can be prunable during that wave anyway: an entry only becomes prunable a whole
// interval after it was added.
func TestSkewMonitor_AFleetReportingAtOnceDoesNotSweepPerPeer(t *testing.T) {
	req := require.New(t)

	m := NewSkewMonitor(10 * time.Minute)
	skewed := time.Now().Add(-5 * time.Minute).UnixNano()

	// An entry that a sweep would remove, so whether one ran is visible in the map rather than needing to
	// be counted.
	m.Observe("early", skewed)
	m.lock.Lock()
	m.lastWarnFor["early"] = time.Now().Add(-2 * skewReportInterval)
	m.lock.Unlock()

	for i := 0; i < 500; i++ {
		m.Observe(fmt.Sprintf("router-%d", i), skewed)
	}

	m.lock.Lock()
	_, survived := m.lastWarnFor["early"]
	reported := len(m.lastWarnFor)
	m.lock.Unlock()

	req.Equal(501, reported, "every peer in the wave is reported once")
	req.True(survived, "a prunable entry surviving the wave is what shows no sweep ran during it")

	// Once the interval has passed, the next report sweeps and takes it.
	m.lock.Lock()
	m.nextPrune = time.Now().Add(-time.Second)
	m.lock.Unlock()

	m.Observe("later", skewed)

	m.lock.Lock()
	defer m.lock.Unlock()
	_, survived = m.lastWarnFor["early"]
	req.False(survived, "the gate delays the sweep, it does not cancel it")
}
