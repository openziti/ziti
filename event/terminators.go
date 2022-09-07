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
	TerminatorEventsNs = "fabric.terminators"

	TerminatorCreated       TerminatorEventType = "created"
	TerminatorUpdated       TerminatorEventType = "updated"
	TerminatorDeleted       TerminatorEventType = "deleted"
	TerminatorRouterOnline  TerminatorEventType = "router-online"
	TerminatorRouterOffline TerminatorEventType = "router-offline"
)

type TerminatorEvent struct {
	Namespace                 string              `json:"namespace"`
	EventType                 TerminatorEventType `json:"event_type"`
	Timestamp                 time.Time           `json:"timestamp"`
	ServiceId                 string              `json:"service_id"`
	TerminatorId              string              `json:"terminator_id"`
	RouterId                  string              `json:"router_id"`
	HostId                    string              `json:"host_id"`
	RouterOnline              bool                `json:"router_online"`
	Precedence                string              `json:"precedence"`
	StaticCost                uint16              `json:"static_cost"`
	DynamicCost               uint16              `json:"dynamic_cost"`
	TotalTerminators          int                 `json:"total_terminators"`
	UsableDefaultTerminators  int                 `json:"usable_default_terminators"`
	UsableRequiredTerminators int                 `json:"usable_required_terminators"`
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
