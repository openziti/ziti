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

	"github.com/openziti/channel/v5"
	raftMesh "github.com/openziti/ziti/v2/controller/raft/mesh"
)

// gossipPeerSendTimeout bounds how long a single peer send may block on a full
// or stalled peer tx queue before that copy is dropped. Kept short (1s) because a
// healthy tx queue accepts in sub-millisecond time, so only a genuinely stalled
// peer (e.g. a chaos-dropped ctrl-mesh link) hits it, and dropped peer deltas are
// repaired by anti-entropy — no need to hold the worker longer.
const gossipPeerSendTimeout = time.Second

// NoopMesh is a zero-peer Mesh for single-controller mode. Broadcasts and sends
// are no-ops, and the peer list is always empty.
type NoopMesh struct{}

// NewNoopMesh returns a Mesh that has no peers and silently drops all messages.
func NewNoopMesh() *NoopMesh {
	return &NoopMesh{}
}

func (self *NoopMesh) PeerIds() []string                              { return nil }
func (self *NoopMesh) PeerIdForChannel(channel.Channel) string        { return "" }
func (self *NoopMesh) Send(string, *channel.Message) error            { return nil }
func (self *NoopMesh) Broadcast(*channel.Message)                     {}

// Mesh abstracts the peer-to-peer communication layer used by the gossip store.
type Mesh interface {
	PeerIds() []string
	PeerIdForChannel(ch channel.Channel) string
	Send(peerId string, msg *channel.Message) error
	Broadcast(msg *channel.Message)
}

// RaftMeshAdapter wraps a raft mesh.Mesh to implement the gossip Mesh interface.
type RaftMeshAdapter struct {
	mesh raftMesh.Mesh
}

// NewRaftMeshAdapter creates a Mesh backed by the raft peer mesh.
func NewRaftMeshAdapter(m raftMesh.Mesh) *RaftMeshAdapter {
	return &RaftMeshAdapter{mesh: m}
}

func (self *RaftMeshAdapter) PeerIds() []string {
	peers := self.mesh.GetPeers()
	ids := make([]string, 0, len(peers))
	for _, p := range peers {
		ids = append(ids, string(p.Id))
	}
	return ids
}

// PeerIdForChannel returns the raft server ID of the peer that owns the given channel.
func (self *RaftMeshAdapter) PeerIdForChannel(ch channel.Channel) string {
	for _, p := range self.mesh.GetPeers() {
		if p.Channel == ch {
			return string(p.Id)
		}
	}
	return ""
}

func (self *RaftMeshAdapter) Send(peerId string, msg *channel.Message) error {
	for _, p := range self.mesh.GetPeers() {
		if string(p.Id) == peerId {
			return msg.WithTimeout(gossipPeerSendTimeout).Send(p.Channel)
		}
	}
	return nil // peer not found, skip silently
}

func (self *RaftMeshAdapter) Broadcast(msg *channel.Message) {
	for _, p := range self.mesh.GetPeers() {
		_ = cloneForPeer(msg).WithTimeout(gossipPeerSendTimeout).Send(p.Channel)
	}
}

// cloneForPeer returns a copy of msg safe to hand to a single peer channel.
//
// A peer channel mutates the message's Headers map on its own tx goroutine at
// send time (the channel heartbeater is registered as a Tx transform handler and
// writes a header into the message). Broadcasting one shared message to multiple
// peers therefore races one peer's header write against another peer's marshal
// iteration of the same map, which the Go runtime reports as the fatal
// "concurrent map iteration and map write", killing the controller. Each peer
// must get its own message and Headers map. The body is only read during marshal,
// so it is shared.
func cloneForPeer(msg *channel.Message) *channel.Message {
	clone := channel.NewMessage(msg.ContentType, msg.Body)
	for k, v := range msg.Headers {
		clone.Headers[k] = v
	}
	return clone
}
