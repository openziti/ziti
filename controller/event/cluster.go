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
	"crypto/x509"
	"fmt"
	"time"
)

type ClusterEventType string

const (
	ClusterEventNS = "cluster"

	ClusterPeerConnected    ClusterEventType = "peer.connected"
	ClusterPeerDisconnected ClusterEventType = "peer.disconnected"
	ClusterMembersChanged   ClusterEventType = "members.changed"
	ClusterLeadershipGained ClusterEventType = "leadership.gained"
	ClusterLeadershipLost   ClusterEventType = "leadership.lost"
	ClusterHasLeader        ClusterEventType = "state.has_leader"
	ClusterIsLeaderless     ClusterEventType = "state.is_leaderless"
	ClusterStateReadOnly    ClusterEventType = "state.ro"
	ClusterStateReadWrite   ClusterEventType = "state.rw"
	ClusterPeerNotMember    ClusterEventType = "peer.not_member"
)

// A ClusterPeer represents a controller which is a member of the cluster.
type ClusterPeer struct {
	// The controller id.
	Id string `json:"id,omitempty"`

	// The address at which the controller can be reached.
	Addr string `json:"addr,omitempty"`

	// The version of the controller.
	Version string `json:"version,omitempty"`

	ServerCert []*x509.Certificate `json:"-"`

	// The set of api addresses presented by the controller.
	ApiAddresses map[string][]ApiAddress `json:"apiAddresses"`
}

// An ApiAddress represents an endpoint on a controller. This may include things
// like REST management services and health checks.
type ApiAddress struct {
	// The URL of the API endpoint.
	Url string `json:"url"`

	// The version of the API endpoint. Endpoints are versioned independently of
	// the controller version as these are expected to be stable over long periods.
	Version string `json:"version"`
}

func (self *ClusterPeer) String() string {
	return fmt.Sprintf("id=%v addr=%v version=%v", self.Id, self.Addr, self.Version)
}

// A ClusterEvent marks a change to the controller HA cluster.
// ClusterEvents can be of the following types:
//   - peer.connected - a peer connected
//   - peer.disconnected - a peer disconnected
//   - peer.not_member - a peer connected, but was not a member and didn't join the cluster
//   - members.changed - A peer was added to or removed from the cluster
//   - leadership.gained - The node become the cluster leader
//   - leadership.lost - The node lost cluster leadership
//   - state.has_leader - The cluster gained a leader
//   - state.is_leaderless - The cluster became leaderless
//   - state.ro - The cluster is not accepting state changes, likely due to version mismatches in cluster members
//   - state.rw - The cluster is accepting state changes
//
// Example: Cluster Members Changed Event
//
//	{
//	 "namespace": "cluster",
//	 "event_src_id": "ctrl1",
//	 "timestamp": "2025-01-17T13:41:25.817205826-05:00",
//	 "eventType": "members.changed",
//	 "index": 7,
//	 "peers": [
//	   {
//	     "id": "ctrl1",
//	     "addr": "tls:localhost:6262",
//	     "apiAddresses": null
//	   },
//	   {
//	     "id": "ctrl2",
//	     "addr": "tls:localhost:6363",
//	     "apiAddresses": null
//	   }
//	 ]
//	}
//
// Example: Peer Connected Event
//
//	{
//	 "namespace": "cluster",
//	 "event_src_id": "ctrl1",
//	 "timestamp": "2025-01-17T13:41:25.838625953-05:00",
//	 "eventType": "peer.connected",
//	 "peers": [
//	   {
//	     "id": "ctrl2",
//	     "addr": "tls:localhost:6363",
//	     "version": "v0.0.0",
//	     "apiAddresses": {
//	       "edge-client": [
//	         {
//	           "url": "https://127.0.0.1:1380/edge/client/v1",
//	           "version": "v1"
//	         }
//	       ],
//	       "edge-management": [
//	         {
//	           "url": "https://127.0.0.1:1380/edge/management/v1",
//	           "version": "v1"
//	         }
//	       ],
//	       "edge-oidc": [
//	         {
//	           "url": "https://127.0.0.1:1380/oidc",
//	           "version": "v1"
//	         }
//	       ],
//	       "fabric": [
//	         {
//	           "url": "https://127.0.0.1:1380/fabric/v1",
//	           "version": "v1"
//	         }
//	       ],
//	       "health-checks": [
//	         {
//	           "url": "https://127.0.0.1:1380/health-checks",
//	           "version": "v1"
//	         }
//	       ]
//	     }
//	   }
//	 ]
//	}
type ClusterEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The cluster event type. See above for set of valid types.
	EventType ClusterEventType `json:"eventType"`

	// The raft index associated with the event.
	Index uint64 `json:"index,omitempty"`

	// This field is populated with all peers when membership change events
	// or leadership is gained. It is populated with the connecting peer for connect events and the
	// disconnecting peer for disconnect events. For other types it is omitted.
	Peers []*ClusterPeer `json:"peers,omitempty"`

	// The leader id. Only populated for state.has_leader events.
	LeaderId string `json:"leaderId,omitempty"`
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
		Namespace: ClusterEventNS,
		EventType: eventType,
		Timestamp: time.Now(),
	}
}
