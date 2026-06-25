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
	"github.com/michaelquigley/pfxlog"
)

// PeerConnected sends a full snapshot of all stateMaps to the newly connected peer.
func (s *Store) PeerConnected(peerId string) {
	log := pfxlog.Logger().WithField("peer", peerId)
	log.Info("sending gossip snapshot to newly connected peer")

	s.types.Range(func(key, value any) bool {
		sm := value.(*stateMap)
		sm.sendSnapshotTo(peerId)
		return true
	})
}

// PeerDisconnected cleans up pending confirmed-propagation waiters for the given peer.
func (s *Store) PeerDisconnected(peerId string) {
	pfxlog.Logger().WithField("peer", peerId).Info("cleaning up gossip state for disconnected peer")
	s.cleanupPendingAcks(peerId)
}
