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
	LinkEventsNs = "fabric.links"

	LinkDialed                     LinkEventType = "dialed"
	LinkFault                      LinkEventType = "fault"
	LinkConnected                  LinkEventType = "connected"
	LinkFromRouterNew              LinkEventType = "routerLinkNew"
	LinkFromRouterKnown            LinkEventType = "routerLinkKnown"
	LinkFromRouterDisconnectedDest LinkEventType = "routerLinkDisconnectedDest"
)

type LinkConnection struct {
	Id         string `json:"id"`
	LocalAddr  string `json:"local_addr"`
	RemoteAddr string `json:"remote_addr"`
}

type LinkEvent struct {
	Namespace   string            `json:"namespace"`
	EventType   LinkEventType     `json:"event_type"`
	Timestamp   time.Time         `json:"timestamp"`
	LinkId      string            `json:"link_id"`
	SrcRouterId string            `json:"src_router_id"`
	DstRouterId string            `json:"dst_router_id"`
	Protocol    string            `json:"protocol"`
	DialAddress string            `json:"dial_address"`
	Cost        int32             `json:"cost"`
	Connections []*LinkConnection `json:"connections,omitempty"`
}

func (event *LinkEvent) String() string {
	return fmt.Sprintf("%v.%v time=%v linkId=%v srcRouterId=%v dstRouterId=%v",
		event.Namespace, event.EventType, event.Timestamp, event.LinkId, event.SrcRouterId, event.DstRouterId)
}

type LinkEventHandler interface {
	AcceptLinkEvent(event *LinkEvent)
}
