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

type TerminatorEventType string

const (
	TerminatorEventNS = "terminator"

	TerminatorCreated       TerminatorEventType = "created"
	TerminatorUpdated       TerminatorEventType = "updated"
	TerminatorDeleted       TerminatorEventType = "deleted"
	TerminatorRouterOnline  TerminatorEventType = "router-online"
	TerminatorRouterOffline TerminatorEventType = "router-offline"
)

// A TerminatorEvent is emitted at various points in the terminator lifecycle.
//
// Note: In version prior to 1.4.0, the namespace was `fabric.terminators`
//
// Valid values for terminator event types are:
//   - created - Note: replaced by entity change events
//   - updated - Note: replaced by entity changes events
//   - deleted - Note: replaced by entity change events
//   - router-online
//   - router-offline
//
// Example: Terminator created event
//
//	{
//	 "namespace": "terminator",
//	 "event_type": "created",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:35:33.691240129-05:00",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP",
//	 "terminator_id": "2c9DGllUFx2GIFWFF5g5FP",
//	 "router_id": "5g2QrZxFcw",
//	 "host_id": "IahyE.5Scw",
//	 "router_online": true,
//	 "precedence": "default",
//	 "static_cost": 0,
//	 "dynamic_cost": 0,
//	 "total_terminators": 1,
//	 "usable_default_terminators": 1,
//	 "usable_required_terminators": 0
//	}
//
// Example: Terminator router offline event
//
//	{
//	 "namespace": "terminator",
//	 "event_type": "router-offline",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:35:41.120951142-05:00",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP",
//	 "terminator_id": "2c9DGllUFx2GIFWFF5g5FP",
//	 "router_id": "5g2QrZxFcw",
//	 "host_id": "IahyE.5Scw",
//	 "router_online": false,
//	 "precedence": "default",
//	 "static_cost": 0,
//	 "dynamic_cost": 0,
//	 "total_terminators": 1,
//	 "usable_default_terminators": 0,
//	 "usable_required_terminators": 0
//	}
//
// Example: Terminator router online event
//
//	{
//	 "namespace": "terminator",
//	 "event_type": "router-online",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:35:42.438815052-05:00",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP",
//	 "terminator_id": "2c9DGllUFx2GIFWFF5g5FP",
//	 "router_id": "5g2QrZxFcw",
//	 "host_id": "IahyE.5Scw",
//	 "router_online": true,
//	 "precedence": "default",
//	 "static_cost": 0,
//	 "dynamic_cost": 0,
//	 "total_terminators": 1,
//	 "usable_default_terminators": 1,
//	 "usable_required_terminators": 0
//	}
//
// Example: Terminator Deleted Event
//
//	{
//	 "namespace": "terminator",
//	 "event_type": "deleted",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:35:42.448238469-05:00",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP",
//	 "terminator_id": "2c9DGllUFx2GIFWFF5g5FP",
//	 "router_id": "5g2QrZxFcw",
//	 "host_id": "IahyE.5Scw",
//	 "router_online": true,
//	 "precedence": "default",
//	 "static_cost": 0,
//	 "dynamic_cost": 0,
//	 "total_terminators": 0,
//	 "usable_default_terminators": 0,
//	 "usable_required_terminators": 0
//	}
type TerminatorEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The type of terminator event. For valid values see above.
	EventType TerminatorEventType `json:"event_type"`

	// The id of the service that this terminator belongs to.
	ServiceId string `json:"service_id"`

	// The terminator's identifier.
	TerminatorId string `json:"terminator_id"`

	// The id of router that the terminator lives on.
	RouterId string `json:"router_id"`

	// Optional identifier indicating what is hosting the terminator. If hosted by the edge, will be an identity id.
	HostId string `json:"host_id"`

	// Indicates if the terminator's router is online.
	RouterOnline bool `json:"router_online"`

	// The terminator precedence.
	Precedence string `json:"precedence"`

	// The terminator's static cost.
	StaticCost uint16 `json:"static_cost"`

	// The terminator's dynamic cost (usually based on the number of circuits currently on the terminator).
	DynamicCost uint16 `json:"dynamic_cost"`

	// The total number of terminators that the service has.
	TotalTerminators int `json:"total_terminators"`

	// The number of online terminators with a default precedence for the service.
	UsableDefaultTerminators int `json:"usable_default_terminators"`

	// The number of online terminators with a required precedence for the service.
	UsableRequiredTerminators int `json:"usable_required_terminators"`

	// For internal use.
	PropagateIndicator bool `json:"-"`
}

func (event *TerminatorEvent) IsModelEvent() bool {
	return event.EventType == TerminatorCreated ||
		event.EventType == TerminatorUpdated ||
		event.EventType == TerminatorDeleted
}

func (event *TerminatorEvent) String() string {
	return fmt.Sprintf("%v.%v time=%v serviceId=%v terminatorId=%v routerId=%v routerOnline=%v precedence=%v "+
		"staticCost=%v dynamicCost=%v totalTerminators=%v usableDefaultTerminator=%v usableRequiredTerminators=%v",
		event.Namespace, event.EventType, event.Timestamp, event.ServiceId, event.TerminatorId, event.RouterId, event.RouterOnline,
		event.Precedence, event.StaticCost, event.DynamicCost, event.TotalTerminators, event.UsableDefaultTerminators,
		event.UsableRequiredTerminators)
}

type TerminatorEventHandler interface {
	AcceptTerminatorEvent(event *TerminatorEvent)
}

type TerminatorEventHandlerWrapper interface {
	TerminatorEventHandler
	IsWrapping(value TerminatorEventHandler) bool
}
