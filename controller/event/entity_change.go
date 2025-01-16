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

import "time"

type EntityChangeEventType string

const (
	EntityChangeEventNS = "entityChange"

	EntityChangeTypeEntityCreated EntityChangeEventType = "created"
	EntityChangeTypeEntityUpdated EntityChangeEventType = "updated"
	EntityChangeTypeEntityDeleted EntityChangeEventType = "deleted"
	EntityChangeTypeCommitted     EntityChangeEventType = "committed"
)

// An EntityChangeEvent is emitted when a entity in the data model changes.
//
// Valid entity change event types are:
//   - created
//   - updated
//   - deleted
//   - committed
//
// Entity change events happen in two parts. First the created,updated or deleted event is
// emitted. This happens inside a transaction. After the transaction is committed, a
// committed event with the same event id is generated. This lets the event consumer know
// that the event is finalized. All changes within a transaction will share the same event
// id. If a new event id is seen before the previous event is committed, that indicates
// that the transaction was rolled back.
//
// Example: Service Created Event
//
//	{
//	  "namespace": "entityChange",
//	  "event_src_id": "ctrl1",
//	  "timestamp": "2023-05-11T21:41:47.128588927-04:00",
//	  "eventId": "326faf6c-8123-42ae-9ed8-6fd9560eb567",
//	  "eventType": "created",
//	  "metadata": {
//	    "author": {
//	       "type": "identity",
//	       "id": "ji2Rt8KJ4",
//	       "name": "Default Admin"
//	    },
//	    "source": {
//	       "type": "rest",
//	       "auth": "edge",
//	       "localAddr": "localhost:1280",
//	       "remoteAddr": "127.0.0.1:37578",
//	       "method": "POST"
//	    },
//	    "version": "v0.0.0"
//	  },
//	  "entityType": "services",
//	  "isParentEvent": false,
//	  "initialState": null,
//	  "finalState": {
//	     "id": "6S0bCGWb6yrAutXwSQaLiv",
//	     "createdAt": "2023-05-12T01:41:47.128138887Z",
//	     "updatedAt": "2023-05-12T01:41:47.128138887Z",
//	     "tags": {},
//	     "isSystem": false,
//	     "name": "test",
//	     "terminatorStrategy": "smartrouting",
//	     "roleAttributes": [
//	       "goodbye",
//	       "hello"
//	     ],
//	     "configs": null,
//	     "encryptionRequired": true
//	  }
//	}
//
// Example: Change Committed Event
//
//	{
//	  "namespace": "entityChange",
//	  "event_src_id": "ctrl1",
//	  "timestamp": "2023-05-11T21:41:47.129235443-04:00"
//	  "eventId": "326faf6c-8123-42ae-9ed8-6fd9560eb567",
//	  "eventType": "committed",
//	}
type EntityChangeEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// An identifier shared by all changes in a given transaction.
	EventId string `json:"eventId"`

	// The entity change event type. See above for valid values.
	EventType EntityChangeEventType `json:"eventType"`

	// Metadata will include information about who initiated the change and how.
	Metadata map[string]any `json:"metadata,omitempty"`

	// The type of the entity being changed.
	EntityType string `json:"entityType,omitempty"`

	// True if the entity type has a parent type (like services and routers), and
	// this event only contains the parent data.
	IsParentEvent *bool `json:"isParentEvent,omitempty"`

	// The state before the change. Will be empty for creates.
	InitialState any `json:"initialState,omitempty"`

	// The state after the change. Will be empty for deletes.
	FinalState any `json:"finalState,omitempty"`

	PropagateIndicator bool `json:"-"`
	IsRecoveryEvent    bool `json:"-"`
}

type EntityChangeEventHandler interface {
	AcceptEntityChangeEvent(event *EntityChangeEvent)
}

type EntityChangeEventHandlerWrapper interface {
	EntityChangeEventHandler
	IsWrapping(value EntityChangeEventHandler) bool
}
