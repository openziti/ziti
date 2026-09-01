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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"google.golang.org/protobuf/proto"
)

// runAntiEntropy periodically sends a digest to a rotating peer so that missed
// deltas can be repaired.
func runAntiEntropy(sm *stateMap, store *Store) {
	ticker := time.NewTicker(sm.config.antiEntropyInterval)
	defer ticker.Stop()

	peerIdx := 0

	for {
		select {
		case <-ticker.C:
			peers := store.mesh.PeerIds()
			if len(peers) == 0 {
				continue
			}

			peerIdx = peerIdx % len(peers)
			targetPeer := peers[peerIdx]
			peerIdx++

			body, err := proto.Marshal(sm.buildAntiEntropyDigest())
			if err != nil {
				pfxlog.Logger().WithError(err).Error("failed to marshal gossip digest for anti-entropy")
				continue
			}
			msg := channel.NewMessage(gossip_pb.GossipDigestType, body)
			if err := store.mesh.Send(targetPeer, msg); err != nil {
				pfxlog.Logger().WithError(err).WithField("peer", targetPeer).
					Warn("failed to send anti-entropy digest")
			}

		case <-store.closeCh:
			return
		}
	}
}

// buildAntiEntropyDigest builds the message that opens an anti-entropy round with a peer.
//
// It carries one hash per owner and no per-entry versions. The peer only has to find the owners the two
// disagree about, and a hash answers that; a version per entry answers it more precisely at a cost that does
// not hold up at scale. In a full mesh every router owns an entry per link, so the per-entry form is a
// multi-megabyte message every interval, marshalled and then walked into a same-sized map by the receiver,
// almost always to conclude that nothing has diverged. Precision within a diverged owner is given up
// instead: see entriesNeededByPeer for what that costs.
//
// An empty digest is meaningful rather than pointless, and is still sent: it tells the peer this controller
// holds nothing, which is exactly the state a restarted one needs to advertise to be sent everything.
func (sm *stateMap) buildAntiEntropyDigest() *gossip_pb.GossipDigest {
	return &gossip_pb.GossipDigest{
		StoreType:    sm.name,
		OwnerDigests: sm.getOwnerDigests(),
		SentAt:       time.Now().UnixNano(),
	}
}

// entriesNeededByPeer returns the entries this controller holds that the peer which sent digest is missing or
// holds at an older version. The answer only ever covers owners this controller knows about; entries only the
// peer holds are its own business, and are repaired when it runs its own round in the other direction.
//
// An owner whose hash matches is byte-identical on both sides and is skipped without reading an entry. That
// is the steady state, and it is what makes the exchange cheap.
//
// An owner whose hash differs, or that the peer did not list at all, is sent in full. The hash says the two
// views disagree but not where, and narrowing it would cost another round trip to fetch the peer's versions.
// Sending the owner's whole set instead means the peer re-applies entries it already has and rejects them on
// version; that redundancy is bounded by one owner's entries and is counted against the anti-entropy meters,
// so it does not disturb the broadcast path's stale rate.
//
// A digest that does carry per-entry versions (a peer still running the older exchange) is honoured: those
// owners are narrowed to just the entries the peer is behind on.
func (sm *stateMap) entriesNeededByPeer(digest *gossip_pb.GossipDigest) []*gossip_pb.GossipEntry {
	remoteVersions := make(map[string]uint64, len(digest.Entries))
	for _, de := range digest.Entries {
		remoteVersions[de.Key] = de.Version
	}

	var ownerHashes map[string]uint64
	if len(digest.OwnerDigests) > 0 {
		ownerHashes = make(map[string]uint64, len(digest.OwnerDigests))
		for _, od := range digest.OwnerDigests {
			ownerHashes[od.Owner] = od.Hash
		}
	}

	// First pass: collect (owner, ownerData) pairs under the cmap iteration lock without calling anything
	// that would re-enter cmap. Calling hashForOwnerFull inside the IterCb callback would re-acquire the
	// shard RLock; under writer-priority RWMutex semantics, with a writer queued, that re-acquire deadlocks
	// against the outer RLock.
	type ownerEntry struct {
		owner string
		od    *ownerData
	}
	var ownersList []ownerEntry
	sm.owners.IterCb(func(owner string, od *ownerData) {
		ownersList = append(ownersList, ownerEntry{owner: owner, od: od})
	})

	var needed []*gossip_pb.GossipEntry
	var matched, diffed int64
	for _, oe := range ownersList {
		if ownerHashes != nil {
			if remoteHash, ok := ownerHashes[oe.owner]; ok {
				// hashForOwnerFull (not hashForOwner) is required here: the live-only hash matches across
				// controllers that differ on tombstones, so anti-entropy would silently suppress tombstone
				// divergence repair.
				if sm.hashForOwnerFull(oe.owner) == remoteHash {
					matched++
					continue
				}
				diffed++
			}
			// Owner not in the peer's hash map: it knows nothing about this owner, so everything held for
			// the owner is new to it. The entries map lookups below see !exists and send all of them.
		}
		oe.od.mu.RLock()
		for key, e := range oe.od.entries {
			remoteVersion, exists := remoteVersions[key]
			if !exists || e.Version > remoteVersion {
				needed = append(needed, e.toProto())
			}
		}
		oe.od.mu.RUnlock()
	}
	sm.markAntiEntropyOwnersMatched(matched)
	sm.markAntiEntropyOwnersDiffed(diffed)

	return needed
}
