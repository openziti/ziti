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

package event

import (
	"fmt"
	"time"
)

type ClusterEventType string

const (
	ClusterEventsNs = "fabric.cluster"

	ClusterPeerConnected    ClusterEventType = "peer.connected"
	ClusterPeerDisconnected ClusterEventType = "peer.disconnected"
	ClusterMembersChanged   ClusterEventType = "members.changed"
	ClusterLeadershipGained ClusterEventType = "leadership.gained"
	ClusterLeadershipLost   ClusterEventType = "leadership.lost"
	ClusterStateReadOnly    ClusterEventType = "state.ro"
	ClusterStateReadWrite   ClusterEventType = "state.rw"
)

type ClusterPeer struct {
	Id      string `json:"id,omitempty"`
	Addr    string `json:"addr,omitempty"`
	Version string `json:"version,omitempty"`
}

func (self *ClusterPeer) String() string {
	return fmt.Sprintf("id=%v addr=%v version=%v", self.Id, self.Addr, self.Version)
}

type ClusterEvent struct {
	Namespace string           `json:"namespace"`
	EventType ClusterEventType `json:"eventType"`
	Timestamp time.Time        `json:"timestamp"`
	Index     uint64           `json:"index,omitempty"`
	Peers     []*ClusterPeer   `json:"peers,omitempty"`
}

func (event *ClusterEvent) String() string {
	return fmt.Sprintf("%v.%v time=%v peers=%v", event.Namespace, event.EventType, event.Timestamp, event.Peers)
}

type ClusterEventHandler interface {
	AcceptClusterEvent(event *ClusterEvent)
}

type ClusterEventHandlerF func(event *ClusterEvent)

func (f ClusterEventHandlerF) AcceptClusterEvent(event *ClusterEvent) {
	f(event)
}

func NewClusterEvent(eventType ClusterEventType) *ClusterEvent {
	return &ClusterEvent{
		Namespace: ClusterEventsNs,
		EventType: eventType,
		Timestamp: time.Now(),
	}
}
