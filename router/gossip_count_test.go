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

package router

import (
	"testing"

	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/stretchr/testify/require"
)

// Test_gossipStore_countIsDerived guards against reintroducing an incrementally
// maintained entry counter, which drifted from currentEntries and left routers
// permanently off-by-one against the controller. count() must always equal the
// live (non-tombstone) entry set, regardless of the mutation sequence, and must
// exclude any tombstone present in the map (matching the hash).
func Test_gossipStore_countIsDerived(t *testing.T) {
	req := require.New(t)
	s := newGossipStore(nil)

	setLive := func(key string, version uint64) {
		s.currentEntries.Set(key, &gossip_pb.GossipEntry{Key: key, Version: version})
		s.invalidateHash()
	}
	setTombstone := func(key string) {
		s.currentEntries.Set(key, &gossip_pb.GossipEntry{Key: key, Tombstone: true})
		s.invalidateHash()
	}
	remove := func(key string) {
		s.currentEntries.Remove(key)
		s.invalidateHash()
	}
	liveInMap := func() int64 {
		var n int64
		s.currentEntries.IterCb(func(_ string, e *gossip_pb.GossipEntry) {
			if !e.Tombstone {
				n++
			}
		})
		return n
	}
	checkConsistent := func() {
		req.Equal(liveInMap(), s.count(), "count() must equal the live entry set")
	}

	setLive("a", 1)
	setLive("b", 1)
	setLive("c", 1)
	checkConsistent()
	req.Equal(int64(3), s.count())

	// Re-set an existing key (e.g. a new version): must not double-count.
	setLive("a", 2)
	checkConsistent()
	req.Equal(int64(3), s.count())

	remove("b")
	checkConsistent()
	req.Equal(int64(2), s.count())

	// A tombstone in the map must not be counted (matches hash's filter).
	setTombstone("d")
	checkConsistent()
	req.Equal(int64(2), s.count())

	// Removing an absent key is a no-op.
	remove("zzz")
	checkConsistent()
	req.Equal(int64(2), s.count())
}
