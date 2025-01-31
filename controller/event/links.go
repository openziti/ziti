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

type LinkEventType string

const (
	LinkEventNS = "link"

	LinkFault                      LinkEventType = "fault"
	LinkDuplicate                  LinkEventType = "duplicate"
	LinkFromRouterNew              LinkEventType = "routerLinkNew"
	LinkFromRouterKnown            LinkEventType = "routerLinkKnown"
	LinkFromRouterDisconnectedDest LinkEventType = "routerLinkDisconnectedDest"

	// LinkDialed is only used when legacy controller link management is enabled
	LinkDialed LinkEventType = "dialed"

	// LinkConnected is only used when legacy controller link management is enabled
	LinkConnected LinkEventType = "connected"
)

// A LinkConnection describes a physical connection that forms a link. A Link may be made
// up of multiple LinkConnections.
//
// Link ids currently have three valid values:
//   - single - meaning the link has a single connection
//   - payload - a connection used only for payloads
//   - ack - a connection used only for acks
type LinkConnection struct {
	// The connection identifier.
	Id string `json:"id"`

	// The connection address on dialing router side.
	LocalAddr string `json:"local_addr"`

	// The connection address on accepting router side.
	RemoteAddr string `json:"remote_addr"`
}

// A LinkEvent will be emitted for various link lifecycle events.
//
// Note: In version prior to 1.4.0, the namespace was `fabric.links`
//
// Valid values for link event types are:
//   - routerLinkNew - A router established a new link, or is syncing the controller with link information after a restart/reconnect.
//   - fault - a link has closed due to a link fault
//   - duplicate - a link was removed because it was a duplicate. Happens when routers dial each other at the same time.
//   - routerLinkKnown - A router informed the controller of a link, but the controller already knew about it.
//   - routerLinkDisconnectedDest - A router created a link, but the destination router isn't currently connected to the controller.
//   - dialed - Deprecated. Happens when a link listener has been dialed. Only relevant if using legacy controller managed links.
//   - connected - Deprecated. Happens when a link is connected. Only generated when using legacy controller managed links.
//
// Example: Router Link New Event
//
//	{
//	 "namespace": "fabric.links",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-23T16:19:26.879417243-05:00",
//	 "event_type": "routerLinkNew",
//	 "link_id": "2Wea35TgSfWbh3P1wmEyLd",
//	 "src_router_id": "5g2QrZxFcw",
//	 "dst_router_id": "E0IAgBVdmn",
//	 "protocol": "dtls",
//	 "dial_address": "dtls:127.0.0.1:4024",
//	 "cost": 1
//	}
//
// Example: Link Faulted Event
//
//	{
//	  "namespace": "link",
//	  "event_src_id": "ctrl1",
//	  "timestamp": "2022-07-15T18:10:19.973867809-04:00",
//	  "event_type": "fault",
//	  "link_id": "6slUYCqOB85YTfdiD8I5pl",
//	  "src_router_id": "YPpTEd8JP",
//	  "dst_router_id": "niY.XmLArx",
//	  "protocol": "tls",
//	  "dial_address": "tls:127.0.0.1:4023",
//	  "cost": 1
//	}
//
// Example: Router Link Known Event
//
//	{
//	 "namespace": "link",
//	 "event_src_id": "ctrl1",
//	 "timestamp": "2022-07-15T18:10:19.974177638-04:00",
//	 "event_type": "routerLinkKnown",
//	 "link_id": "47kGIApCXI29VQoCA1xXWI",
//	 "src_router_id": "niY.XmLArx",
//	 "dst_router_id": "YPpTEd8JP",
//	 "protocol": "tls",
//	 "dial_address": "tls:127.0.0.1:4024",
//	 "cost": 1
//	}
type LinkEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The link event type. See above for valid values.
	EventType LinkEventType `json:"event_type"`

	// The link identifier.
	LinkId string `json:"link_id"`

	// The id of the dialing router.
	SrcRouterId string `json:"src_router_id"`

	// The id of the accepting router.
	DstRouterId string `json:"dst_router_id"`

	// The link protocol.
	Protocol string `json:"protocol"`

	// The address dialed.
	DialAddress string `json:"dial_address"`

	// The link cost.
	Cost int32 `json:"cost"`

	// The connections making up the link.
	Connections []*LinkConnection `json:"connections,omitempty"`
}

func (event *LinkEvent) String() string {
	return fmt.Sprintf("%v.%v time=%v linkId=%v srcRouterId=%v dstRouterId=%v",
		event.Namespace, event.EventType, event.Timestamp, event.LinkId, event.SrcRouterId, event.DstRouterId)
}

type LinkEventHandler interface {
	AcceptLinkEvent(event *LinkEvent)
}

type LinkEventHandlerWrapper interface {
	LinkEventHandler
	IsWrapping(value LinkEventHandler) bool
}
