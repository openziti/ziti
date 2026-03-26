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
	"github.com/openziti/channel/v4"
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

			digest := sm.getDigest()
			pbDigest := &gossip_pb.GossipDigest{
				StoreType: sm.name,
				Entries:   digest,
			}
			body, err := proto.Marshal(pbDigest)
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
