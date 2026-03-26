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
	"context"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"github.com/teris-io/shortid"
	"google.golang.org/protobuf/proto"
)

type pendingAck struct {
	remaining map[string]struct{} // peer IDs we're still waiting for
	done      chan struct{}
	mu        sync.Mutex
}

func (pa *pendingAck) ack(peerId string) {
	pa.mu.Lock()
	defer pa.mu.Unlock()
	delete(pa.remaining, peerId)
	if len(pa.remaining) == 0 {
		select {
		case <-pa.done:
		default:
			close(pa.done)
		}
	}
}

// setConfirmed stores a value and waits for acks from all current peers.
func (s *Store) setConfirmed(ctx context.Context, sm *stateMap, key, owner string, value []byte) error {
	version := s.nextVersion()

	e := &entry{
		Key:       key,
		Value:     value,
		Version:   version,
		Owner:     owner,
		UpdatedAt: time.Now(),
	}

	var wasSet bool
	var isCreate bool
	sm.entries.Upsert(key, e, func(exist bool, existing *entry, newValue *entry) *entry {
		if exist && existing.Version >= version {
			return existing
		}
		wasSet = true
		isCreate = !exist || existing.Tombstone
		return e
	})

	if !wasSet {
		return nil
	}

	sm.invalidateOwnerHash(owner)
	sm.updateOwnerStats(owner, version, false, isCreate)

	if sm.listener != nil {
		sm.listener.entryChanged(key, value, version, owner, isCreate, OriginLocal)
	}

	peers := s.mesh.PeerIds()
	if len(peers) == 0 {
		return nil
	}

	requestId, err := shortid.Generate()
	if err != nil {
		return err
	}

	remaining := make(map[string]struct{}, len(peers))
	for _, p := range peers {
		remaining[p] = struct{}{}
	}

	pa := &pendingAck{
		remaining: remaining,
		done:      make(chan struct{}),
	}
	s.pendingAcks.Store(requestId, pa)
	defer s.pendingAcks.Delete(requestId)

	delta := &gossip_pb.GossipDelta{
		StoreType:    sm.name,
		Entries:      []*gossip_pb.GossipEntry{e.toProto()},
		RequestId:    requestId,
		AckRequested: true,
	}
	body, marshalErr := proto.Marshal(delta)
	if marshalErr != nil {
		return marshalErr
	}
	msg := channel.NewMessage(gossip_pb.GossipDeltaType, body)
	s.mesh.Broadcast(msg)

	select {
	case <-pa.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closeCh:
		return context.Canceled
	}
}

// resolveAck is called when an ack message is received from a peer.
func (s *Store) resolveAck(peerId string, requestId string) {
	v, ok := s.pendingAcks.Load(requestId)
	if !ok {
		return
	}
	pa := v.(*pendingAck)
	pa.ack(peerId)
}

// cleanupPendingAcks resolves all pending acks for a disconnected peer.
func (s *Store) cleanupPendingAcks(peerId string) {
	s.pendingAcks.Range(func(key, value any) bool {
		pa := value.(*pendingAck)
		pa.ack(peerId)
		return true
	})
}

func init() {
	sid, err := shortid.New(1, shortid.DefaultABC, 2342)
	if err != nil {
		pfxlog.Logger().WithError(err).Warn("failed to init shortid for gossip confirmed propagation")
	} else {
		shortid.SetDefault(sid)
	}
}
