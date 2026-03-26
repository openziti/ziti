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
	"github.com/openziti/channel/v4"
	raftMesh "github.com/openziti/ziti/v2/controller/raft/mesh"
)

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
			return msg.Send(p.Channel)
		}
	}
	return nil // peer not found, skip silently
}

func (self *RaftMeshAdapter) Broadcast(msg *channel.Message) {
	for _, p := range self.mesh.GetPeers() {
		_ = msg.Send(p.Channel)
	}
}
