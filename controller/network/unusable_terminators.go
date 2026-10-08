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

package network

import (
	"sync/atomic"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
)

// unusableTerminatorTTL is how long a terminator reported unusable stays out of path selection unless
// it is unmarked first.
const unusableTerminatorTTL = time.Minute

// unusableTerminators holds the ids of terminators a router has reported invalid or unusable, so path
// selection can skip them. An entry lasts until Unmark or until its ttl passes. It is local soft
// state: each controller learns from its own circuit attempts. Safe for concurrent use.
type unusableTerminators struct {
	ttl       time.Duration
	expiries  cmap.ConcurrentMap[string, time.Time]
	nextSweep atomic.Int64
}

func newUnusableTerminators(ttl time.Duration) *unusableTerminators {
	return &unusableTerminators{
		ttl:      ttl,
		expiries: cmap.New[time.Time](),
	}
}

// Mark excludes terminatorId from path selection for the ttl, restarting it if already marked.
func (self *unusableTerminators) Mark(terminatorId string) {
	now := time.Now()
	self.expiries.Set(terminatorId, now.Add(self.ttl))

	// Entries for terminators that are neither cleared nor looked up again are dropped here.
	next := self.nextSweep.Load()
	if now.UnixNano() >= next && self.nextSweep.CompareAndSwap(next, now.Add(self.ttl).UnixNano()) {
		var expired []string
		self.expiries.IterCb(func(terminatorId string, expiry time.Time) {
			if !now.Before(expiry) {
				expired = append(expired, terminatorId)
			}
		})
		for _, terminatorId := range expired {
			self.expiries.RemoveCb(terminatorId, func(_ string, expiry time.Time, exists bool) bool {
				return exists && !now.Before(expiry)
			})
		}
	}
}

// IsMarked reports whether terminatorId is excluded from path selection.
func (self *unusableTerminators) IsMarked(terminatorId string) bool {
	expiry, found := self.expiries.Get(terminatorId)
	return found && time.Now().Before(expiry)
}

// Unmark makes terminatorId selectable again. It isn't ordered against a Mark from a report that was
// in flight; a lost mark costs one more failed dial, which marks it again, and a stale mark lasts at
// most the ttl.
func (self *unusableTerminators) Unmark(terminatorId string) {
	self.expiries.Remove(terminatorId)
}
