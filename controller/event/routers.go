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
	RouterEventsNs = "fabric.routers"

	RouterOnline  RouterEventType = "router-online"
	RouterOffline RouterEventType = "router-offline"
)

type RouterEvent struct {
	Namespace    string          `json:"namespace"`
	EventType    RouterEventType `json:"event_type"`
	Timestamp    time.Time       `json:"timestamp"`
	RouterId     string          `json:"router_id"`
	RouterOnline bool            `json:"router_online"`
}

func (event *RouterEvent) String() string {
	return fmt.Sprintf("%v.%v time=%v routerId=%v routerOnline=%v",
		event.Namespace, event.EventType, event.Timestamp, event.RouterId, event.RouterOnline)
}

type RouterEventHandler interface {
	AcceptRouterEvent(event *RouterEvent)
}
