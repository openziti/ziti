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
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/pb/gossip_pb"
	"google.golang.org/protobuf/proto"
)

// NewDeltaHandler returns a channel handler for incoming gossip deltas.
func (s *Store) NewDeltaHandler() channel.TypedReceiveHandler {
	return &deltaHandler{store: s}
}

// NewAckHandler returns a channel handler for incoming gossip acks.
func (s *Store) NewAckHandler() channel.TypedReceiveHandler {
	return &ackHandler{store: s}
}

// NewDigestHandler returns a channel handler for incoming gossip digests.
func (s *Store) NewDigestHandler() channel.TypedReceiveHandler {
	return &digestHandler{store: s}
}

// NewDigestResponseHandler returns a channel handler for incoming digest responses.
func (s *Store) NewDigestResponseHandler() channel.TypedReceiveHandler {
	return &digestResponseHandler{store: s}
}

// --- deltaHandler ---

type deltaHandler struct {
	store *Store
}

func (h *deltaHandler) ContentType() int32 {
	return gossip_pb.GossipDeltaType
}

func (h *deltaHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.Logger()

	delta := &gossip_pb.GossipDelta{}
	if err := proto.Unmarshal(msg.Body, delta); err != nil {
		log.WithError(err).Error("failed to unmarshal gossip delta")
		return
	}

	if !h.store.queueOrRun(func() {
		h.store.ApplyPeerDelta(delta)

		if delta.AckRequested && delta.RequestId != "" {
			ack := &gossip_pb.GossipAck{
				StoreType: delta.StoreType,
				RequestId: delta.RequestId,
			}
			body, err := proto.Marshal(ack)
			if err != nil {
				log.WithError(err).Error("failed to marshal gossip ack")
				return
			}
			resp := channel.NewMessage(gossip_pb.GossipAckType, body)
			if err := resp.Send(ch); err != nil {
				log.WithError(err).Error("failed to send gossip ack")
			}
		}
	}) {
		log.Debug("peer events pool full, dropping gossip delta")
	}
}

// --- ackHandler --- (kept inline: just a mutex + map delete, microseconds)

type ackHandler struct {
	store *Store
}

func (h *ackHandler) ContentType() int32 {
	return gossip_pb.GossipAckType
}

func (h *ackHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	ack := &gossip_pb.GossipAck{}
	if err := proto.Unmarshal(msg.Body, ack); err != nil {
		pfxlog.Logger().WithError(err).Error("failed to unmarshal gossip ack")
		return
	}

	peerId := h.store.mesh.PeerIdForChannel(ch)
	if peerId == "" {
		pfxlog.Logger().WithField("channelId", ch.Id()).Warn("received gossip ack from unknown peer channel")
		return
	}
	h.store.resolveAck(peerId, ack.RequestId)
}

// --- digestHandler ---

type digestHandler struct {
	store *Store
}

func (h *digestHandler) ContentType() int32 {
	return gossip_pb.GossipDigestType
}

func (h *digestHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.Logger()

	digest := &gossip_pb.GossipDigest{}
	if err := proto.Unmarshal(msg.Body, digest); err != nil {
		log.WithError(err).Error("failed to unmarshal gossip digest")
		return
	}

	if !h.store.queueOrRun(func() {
		sm := h.store.getStateMap(digest.StoreType)
		if sm == nil {
			log.WithField("storeType", digest.StoreType).Warn("received gossip digest for unknown store type")
			return
		}

		// Build a set of entries the sender is missing or has stale versions of
		remoteVersions := make(map[string]uint64, len(digest.Entries))
		for _, de := range digest.Entries {
			remoteVersions[de.Key] = de.Version
		}

		var needed []*gossip_pb.GossipEntry
		sm.entries.IterCb(func(key string, e *entry) {
			remoteVersion, exists := remoteVersions[key]
			if !exists || e.Version > remoteVersion {
				needed = append(needed, e.toProto())
			}
		})

		if len(needed) == 0 {
			return
		}

		resp := &gossip_pb.GossipDigestResponse{
			StoreType: digest.StoreType,
			Entries:   needed,
		}
		body, err := proto.Marshal(resp)
		if err != nil {
			log.WithError(err).Error("failed to marshal gossip digest response")
			return
		}
		respMsg := channel.NewMessage(gossip_pb.GossipDigestResponseType, body)
		if err := respMsg.Send(ch); err != nil {
			log.WithError(err).Error("failed to send gossip digest response")
		}
	}) {
		log.Debug("peer events pool full, dropping gossip digest")
	}
}

// --- digestResponseHandler ---

type digestResponseHandler struct {
	store *Store
}

func (h *digestResponseHandler) ContentType() int32 {
	return gossip_pb.GossipDigestResponseType
}

func (h *digestResponseHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	log := pfxlog.Logger()

	resp := &gossip_pb.GossipDigestResponse{}
	if err := proto.Unmarshal(msg.Body, resp); err != nil {
		log.WithError(err).Error("failed to unmarshal gossip digest response")
		return
	}

	if !h.store.queueOrRun(func() {
		h.store.ApplyPeerDelta(&gossip_pb.GossipDelta{
			StoreType: resp.StoreType,
			Entries:   resp.Entries,
		})
	}) {
		log.Debug("peer events pool full, dropping gossip digest response")
	}
}
