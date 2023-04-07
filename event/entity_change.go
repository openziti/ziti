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

type EntityChangeEventType string

const (
	EntityChangeEventsNs = "entityChange"

	EntityChangeTypeEntityCreated EntityChangeEventType = "created"
	EntityChangeTypeEntityUpdated EntityChangeEventType = "updated"
	EntityChangeTypeEntityDeleted EntityChangeEventType = "deleted"
	EntityChangeTypeCommitted     EntityChangeEventType = "committed"
)

type EntityChangeEvent struct {
	Namespace          string                `json:"namespace"`
	EventId            string                `json:"event_id"`
	EventType          EntityChangeEventType `json:"event_type"`
	Timestamp          time.Time             `json:"timestamp"`
	Metadata           map[string]any        `json:"metadata,omitempty"`
	EntityType         string                `json:"entity_type,omitempty"`
	IsParentEvent      *bool                 `json:"is_parent_event,omitempty"`
	InitialState       any                   `json:"initial_state,omitempty"`
	FinalState         any                   `json:"final_state,omitempty"`
	PropagateIndicator bool                  `json:"-"`
	IsRecoveryEvent    bool                  `json:"-"`
}

type EntityChangeEventHandler interface {
	AcceptEntityChangeEvent(event *EntityChangeEvent)
}

type EntityChangeEventHandlerWrapper interface {
	EntityChangeEventHandler
	IsWrapping(value EntityChangeEventHandler) bool
}
