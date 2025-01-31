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

type RouterEventType string

const (
	RouterEventNS = "router"

	RouterOnline  RouterEventType = "router-online"
	RouterOffline RouterEventType = "router-offline"
)

// A RouterEvent is generated when a router comes online or goes offline.
//
// Note: In version prior to 1.4.0, the namespace was `fabric.routers`
//
// Valid values for router event type are:
//   - router-online
//   - router-offline
//
// Example: Router online event
//
//	{
//	 "namespace": "router",
//	 "event_src_id": "ctrl1",
//	 "timestamp": "2021-04-22T11:26:31.99299884-04:00",
//	 "event_type": "router-online",
//	 "router_id": "JAoyjafljO",
//	 "router_online": true
//	}
//
// Example: Router offline event
//
//	{
//	 "namespace": "router",
//	 "event_src_id": "ctrl1",
//	 "timestamp": "2021-04-22T11:26:41.335114358-04:00",
//	 "event_type": "router-offline",
//	 "router_id": "JAoyjafljO",
//	 "router_online": false
//	}
type RouterEvent struct {
	Namespace  string    `json:"namespace"`
	Timestamp  time.Time `json:"timestamp"`
	EventSrcId string    `json:"event_src_id"`

	// The router event type.
	EventType RouterEventType `json:"event_type"`

	// The router identifier.
	RouterId string `json:"router_id"`

	// Indicates whether the router is online or not. Redundant given
	// the router event type. Should likely be removed.
	RouterOnline bool `json:"router_online"`
}

func (event *RouterEvent) String() string {
	return fmt.Sprintf("%v.%v time=%v routerId=%v routerOnline=%v",
		event.Namespace, event.EventType, event.Timestamp, event.RouterId, event.RouterOnline)
}

type RouterEventHandler interface {
	AcceptRouterEvent(event *RouterEvent)
}

type RouterEventHandlerWrapper interface {
	RouterEventHandler
	IsWrapping(value RouterEventHandler) bool
}
