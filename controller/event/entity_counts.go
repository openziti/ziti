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

const EntityCountEventNS = "entityCount"

// A EntityCountEvent is emitted on a configurable interval. It contains
// the entity counts for all the entity types in the data model.
//
// Note: In version prior to 1.4.0, the namespace was `edge.entityCounts`
//
// Example: Entity Count Event
//
//	{
//	 "namespace": "entityCount",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-16T20:34:47.281325752-05:00",
//	 "counts": {
//	   "apiSessionCertificates": 0,
//	   "apiSessions": 1,
//	   "authPolicies": 1,
//	   "authenticators": 5,
//	   "cas": 0,
//	   "configTypes": 6,
//	   "configs": 4,
//	   "controllers": 0,
//	   "edgeRouterPolicies": 7,
//	   "enrollments": 11,
//	   "eventualEvents": 0,
//	   "externalJwtSigners": 0,
//	   "identities": 16,
//	   "identityTypes": 2,
//	   "mfas": 0,
//	   "postureCheckTypes": 5,
//	   "postureChecks": 0,
//	   "revocations": 0,
//	   "routers": 7,
//	   "routers.edge": 7,
//	   "serviceEdgeRouterPolicies": 1,
//	   "servicePolicies": 4,
//	   "services": 2,
//	   "services.edge": 2,
//	   "sessions": 0,
//	   "terminators": 0
//	 },
//	 "error": ""
//	}
type EntityCountEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// Map of entity type to the number of entities of that type that currently exist in the data model.
	Counts map[string]int64 `json:"counts"`

	// If an error is encountered while collecting entity counts,
	// it will be reported here.
	Error string `json:"error,omitempty"`
}

func (event *EntityCountEvent) String() string {
	return fmt.Sprintf("%v timestamp=%v counts=%+v err=%v",
		event.Namespace, event.Timestamp, event.Counts, event.Error)
}

type EntityCountEventHandler interface {
	AcceptEntityCountEvent(event *EntityCountEvent)
}

type EntityCountEventHandlerWrapper interface {
	EntityCountEventHandler
	IsWrapping(value EntityCountEventHandler) bool
}
