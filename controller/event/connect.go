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
	"time"
)

type ConnectSource string
type ConnectDestination string

const (
	ConnectEventNS                      = "connect"
	ConnectSourceRouter   ConnectSource = "router"
	ConnectSourcePeer     ConnectSource = "peer"
	ConnectSourceIdentity ConnectSource = "identity"

	ConnectDestinationController ConnectDestination = "ctrl"
	ConnectDestinationRouter     ConnectDestination = "router"
)

// A ConnectEvent is emitted when a connection is made to a ziti controller or router.
//
// Valid source types are:
//   - router - router connecting to a controller or another router)
//   - peer - controller connecting to another controller
//   - identity - identity connecting to a router or controller
//
// Valid destination types are:
//   - ctrl - connection is being made to a controller
//   - router - connection is being made to a router
//
// Example: Identity Connected to Controller Event
//
//	{
//	   "namespace": "connect",
//	   "event_src_id": "ctrl_client",
//	   "timestamp": "2024-10-02T12:17:39.501821249-04:00"
//	   "src_type": "identity",
//	   "src_id": "ji2Rt8KJ4",
//	   "src_addr": "127.0.0.1:59336",
//	   "dst_id": "ctrl_client",
//	   "dst_addr": "localhost:1280/edge/management/v1/edge-routers/2L7NeVuGBU",
//	}
//
// Example: Router Connected to Controller Event
//
//	{
//	   "namespace": "connect",
//	   "event_src_id": "ctrl_client",
//	   "timestamp": "2024-10-02T12:17:40.529865849-04:00"
//	   "src_type": "router",
//	   "src_id": "2L7NeVuGBU",
//	   "src_addr": "127.0.0.1:42702",
//	   "dst_id": "ctrl_client",
//	   "dst_addr": "127.0.0.1:6262",
//	}
//
// Example: Controller Connected to Controller Event
//
//	{
//	   "namespace": "connect",
//	   "event_src_id": "ctrl1",
//	   "timestamp": "2024-10-02T12:37:04.490859197-04:00"
//	   "src_type": "peer",
//	   "src_id": "ctrl2",
//	   "src_addr": "127.0.0.1:40056",
//	   "dst_id": "ctrl1",
//	   "dst_addr": "127.0.0.1:6262",
//	}
type ConnectEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The type of software initiating the connection.
	SrcType ConnectSource `json:"src_type"`

	// The type of software receiving the connection.
	DstType ConnectDestination `json:"dst_type"`

	// The id of the initiating component.
	SrcId string `json:"src_id"`

	// The source address of the connection.
	SrcAddr string `json:"src_addr"`

	// The id of the receiving component.
	DstId string `json:"dst_id"`

	// The destination address of the connection.
	DstAddr string `json:"dst_addr"`
}

type ConnectEventHandler interface {
	AcceptConnectEvent(event *ConnectEvent)
}

type ConnectEventHandlerWrapper interface {
	ConnectEventHandler
	IsWrapping(value ConnectEventHandler) bool
}
