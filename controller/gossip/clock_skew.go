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
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
)

const (
	// skewWarnFraction is the share of a tombstone lifetime a clock difference must reach before it is
	// worth reporting. Expressed against the lifetime rather than as a duration because that is what makes
	// skew matter: a deadline one node stamps and another reads only misleads in proportion to how much of
	// the lifetime the difference consumes. Retuning the lifetime retunes this with it.
	skewWarnFraction = 10

	// skewWarnFloor keeps a short lifetime from making every ordinary transit delay a warning.
	skewWarnFloor = 5 * time.Second

	// skewReportInterval bounds how often one peer is reported. Skew persists, so an unbounded report is a
	// log line per exchange for as long as the clocks disagree.
	skewReportInterval = 10 * time.Minute
)

// SkewMonitor reports peers whose wall clock differs from ours by enough to matter.
//
// It reports and takes no corrective action. Adjusting timing from an inferred offset would replace a
// visible, bounded problem with an invisible one, and the measurement is not good enough to act on: a
// one-way delta is skew plus transit and queueing, so it is an upper bound rather than an offset. That is
// adequate for the seconds-to-minutes difference that shifts tombstone collection, and unfit for anything
// finer.
type SkewMonitor struct {
	threshold time.Duration

	lock        sync.Mutex
	lastWarnFor map[string]time.Time
	// nextPrune gates the sweep to once per interval. Without it a fleet sharing one clock offset, which
	// is what an NTP failure looks like and so the case this exists to report, walks the map once per
	// peer as each is seen for the first time: N reports over a map growing to N, under one lock, on the
	// goroutine reading each router's control channel.
	nextPrune time.Time
}

// NewSkewMonitor returns a monitor whose threshold is derived from the given tombstone lifetime. A zero
// lifetime means nothing expires on a deadline, so nothing depends on clocks and the monitor is inert.
func NewSkewMonitor(tombstoneTTL time.Duration) *SkewMonitor {
	if tombstoneTTL <= 0 {
		return nil
	}
	threshold := tombstoneTTL / skewWarnFraction
	if threshold < skewWarnFloor {
		threshold = skewWarnFloor
	}
	return &SkewMonitor{
		threshold:   threshold,
		lastWarnFor: map[string]time.Time{},
		nextPrune:   time.Now().Add(skewReportInterval),
	}
}

// Observe records that a message built at sentAt was received now, warning if the difference is large
// enough to shift when tombstones are collected. sentAt is unix nanoseconds; zero means the sender did not
// stamp one, which is not evidence of agreement and is ignored. A nil monitor observes nothing.
func (self *SkewMonitor) Observe(peer string, sentAt int64) {
	if self == nil || sentAt == 0 {
		return
	}

	now := time.Now()
	delta := now.Sub(time.Unix(0, sentAt))
	magnitude := delta
	if magnitude < 0 {
		magnitude = -magnitude
	}
	if magnitude < self.threshold {
		return
	}

	if !self.shouldReport(peer, now) {
		return
	}

	// Direction is worth naming: a peer ahead of us stamps deadlines we collect late, and a peer behind us
	// stamps deadlines we collect early or that arrive already past.
	direction := "peer clock is behind ours, or the message was slow to arrive"
	if delta < 0 {
		direction = "peer clock is ahead of ours"
	}
	pfxlog.Logger().
		WithField("peer", peer).
		WithField("skewMs", delta.Milliseconds()).
		WithField("thresholdMs", self.threshold.Milliseconds()).
		Warnf("clock difference with peer is large enough to shift tombstone collection: %s. "+
			"This is an upper bound on skew, since it includes transit", direction)
}

// shouldReport rate limits per peer, reporting the first observation for a peer immediately.
//
// Peers are pruned rather than remembered: this map is keyed by peer id, and a deleted router's id never
// returns, since a recreated router is assigned a new one. A fleet that recreates routers therefore hands
// this an unbounded stream of ids over the controller's life. An entry older than the interval it enforces
// suppresses nothing anyway, so it is dropped rather than kept against an id nothing will report under
// again.
func (self *SkewMonitor) shouldReport(peer string, now time.Time) bool {
	self.lock.Lock()
	defer self.lock.Unlock()

	if last, seen := self.lastWarnFor[peer]; seen && now.Sub(last) < skewReportInterval {
		return false
	}

	self.pruneExpiredLocked(now)
	self.lastWarnFor[peer] = now
	return true
}

// pruneExpiredLocked drops peers whose last report is old enough that they would be reported again anyway.
//
// Gated to once per interval rather than run on every added peer. An entry only becomes prunable after a
// whole interval has passed, so sweeping more often than that cannot find anything a single sweep would
// miss, and sweeping per peer makes the first report from a fleet quadratic: each new peer walks a map
// that every previous one just grew.
func (self *SkewMonitor) pruneExpiredLocked(now time.Time) {
	if now.Before(self.nextPrune) {
		return
	}
	self.nextPrune = now.Add(skewReportInterval)

	for peer, last := range self.lastWarnFor {
		if now.Sub(last) >= skewReportInterval {
			delete(self.lastWarnFor, peer)
		}
	}
}
