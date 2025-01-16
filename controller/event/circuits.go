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

type CircuitEventType string

const (
	CircuitEventNS = "circuit"

	CircuitEventsVersion                  = 2
	CircuitCreated       CircuitEventType = "created"
	CircuitUpdated       CircuitEventType = "pathUpdated"
	CircuitDeleted       CircuitEventType = "deleted"
	CircuitFailed        CircuitEventType = "failed"
)

var CircuitEventTypes = []CircuitEventType{CircuitCreated, CircuitUpdated, CircuitDeleted, CircuitFailed}

// A CircuitPath encapsulates information about the circuit's path.
type CircuitPath struct {
	// The routers traversed in the path, going from initiating router to terminating router.
	Nodes []string `json:"nodes"`

	// The links traversed in the path, going from initiating router to terminating router.
	// If the initiating and terminating routers are the same, this will be empty.
	Links []string `json:"links"`

	// The xgress identifier used on the initiating router.
	IngressId string `json:"ingress_id"`

	// The xgress identifier used on the terminating router.
	EgressId string `json:"egress_id"`

	// The local address of the connection to the first ziti component.
	InitiatorLocalAddr string `json:"initiator_local_addr,omitempty"`

	// The remote address of the connection to the first ziti component.
	InitiatorRemoteAddr string `json:"initiator_remote_addr,omitempty"`

	// The local address on the terminating ziti component.
	TerminatorLocalAddr string `json:"terminator_local_addr,omitempty"`

	// The remote address on the terminating ziti component.
	TerminatorRemoteAddr string `json:"terminator_remote_addr,omitempty"`
}

func (self *CircuitPath) String() string {
	if len(self.Nodes) < 1 {
		return "{}"
	}
	if len(self.Links) != len(self.Nodes)-1 {
		return "{malformed}"
	}
	out := fmt.Sprintf("[r/%s]", self.Nodes[0])
	for i := 0; i < len(self.Links); i++ {
		out += fmt.Sprintf("->[l/%s]", self.Links[i])
		out += fmt.Sprintf("->[r/%s]", self.Nodes[i+1])
	}
	return out
}

// A CircuitEvent is emitted for various stages of a circuit lifecycle.
//
// Note: In version prior to 1.4.0, the namespace was `fabric.circuits`
//
// Valid circuit event types are:
//   - created
//   - pathUpdated
//   - deleted
//   - failed
//
// Example: Circuit Created Event
//
//	{
//	 "namespace": "circuit",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T14:09:13.603009739-05:00",
//	 "version": 2,
//	 "event_type": "created",
//	 "circuit_id": "rqrucElFe",
//	 "client_id": "cm614ve9h00fb1xj9dfww20le",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP",
//	 "terminator_id": "7JgrjMgEAis7V5q1wjvoB4",
//	 "instance_id": "",
//	 "creation_timespan": 1035941,
//	 "path": {
//	   "nodes": [
//	     "5g2QrZxFcw"
//	   ],
//	   "links": null,
//	   "ingress_id": "8dN7",
//	   "egress_id": "ZnXG"
//	 },
//	 "link_count": 0,
//	 "path_cost": 262140,
//	 "tags": {
//	   "clientId": "haxn9lB0uc",
//	   "hostId": "IahyE.5Scw",
//	   "serviceId": "3pjMOKY2icS8fkQ1lfHmrP"
//	 }
//	}
//
// Example: Circuit Deleted Event
//
//	{
//	 "namespace": "circuit",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T14:09:15.138049308-05:00",
//	 "version": 2,
//	 "event_type": "deleted",
//	 "circuit_id": "rqrucElFe",
//	 "client_id": "cm614ve9h00fb1xj9dfww20le",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP",
//	 "terminator_id": "7JgrjMgEAis7V5q1wjvoB4",
//	 "instance_id": "",
//	 "path": {
//	   "nodes": [
//	     "5g2QrZxFcw"
//	   ],
//	   "links": null,
//	   "ingress_id": "8dN7",
//	   "egress_id": "ZnXG"
//	 },
//	 "link_count": 0,
//	 "duration": 1535040544,
//	 "tags": {
//	   "clientId": "haxn9lB0uc",
//	   "hostId": "IahyE.5Scw",
//	   "serviceId": "3pjMOKY2icS8fkQ1lfHmrP"
//	 }
//	}
//
// Example: Circuit Failed Event
//
//	{
//	 "namespace": "circuit",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T14:09:30.563045771-05:00",
//	 "version": 2,
//	 "event_type": "failed",
//	 "circuit_id": "JvIucEQHe",
//	 "client_id": "cm614vrcd00fu1xj931hzepec",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP",
//	 "terminator_id": "",
//	 "instance_id": "",
//	 "creation_timespan": 20701,
//	 "path": {
//	   "nodes": null,
//	   "links": null,
//	   "ingress_id": "",
//	   "egress_id": ""
//	 },
//	 "link_count": 0,
//	 "failure_cause": "NO_TERMINATORS",
//	 "tags": {
//	   "clientId": "haxn9lB0uc",
//	   "serviceId": "3pjMOKY2icS8fkQ1lfHmrP"
//	 }
//	}
type CircuitEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The event format version. Currently 2.
	Version uint32 `json:"version"`

	// The circuit event type. See above for valid circuit event types.
	EventType CircuitEventType `json:"event_type"`

	// The circuit id.
	CircuitId string `json:"circuit_id"`

	// Who the circuit was created for. Usually an edge session id.
	ClientId string `json:"client_id"`

	// The id of the circuit's service.
	ServiceId string `json:"service_id"`

	// The terminator the circuit is using.
	TerminatorId string `json:"terminator_id"`

	// The instance id of the terminator.
	InstanceId string `json:"instance_id"`

	// How long it look to create the circuit.
	CreationTimespan *time.Duration `json:"creation_timespan,omitempty"`

	// The circuit's path.
	Path CircuitPath `json:"path"`

	// How many links the circuit is using.
	LinkCount int `json:"link_count"`

	// The circuit's cost at the time of creation or update.
	Cost *uint32 `json:"path_cost,omitempty"`

	// The reason the circuit failed. Only populated for circuit failures.
	FailureCause *string `json:"failure_cause,omitempty"`

	// How long the circuit has been up. Not populated for circuit creates.
	Duration *time.Duration `json:"duration,omitempty"`

	// Contains circuit enrichment data. May contain information like the client and/or host
	// identity ids.
	Tags map[string]string `json:"tags"`
}

func (event *CircuitEvent) String() string {
	return fmt.Sprintf("%v.%v circuitId=%v clientId=%v serviceId=%v path=%v%s",
		event.Namespace, event.EventType, event.CircuitId, event.ClientId, event.ServiceId, event.Path, func() (out string) {
			if event.Path.TerminatorLocalAddr != "" {
				out = fmt.Sprintf("%s (%s)", out, event.Path.TerminatorLocalAddr)
			}
			if event.CreationTimespan != nil {
				out = fmt.Sprintf("%s creationTimespan=%s", out, *event.CreationTimespan)
			}
			return
		}())
}

type CircuitEventHandler interface {
	AcceptCircuitEvent(event *CircuitEvent)
}

type CircuitEventHandlerWrapper interface {
	CircuitEventHandler
	IsWrapping(value CircuitEventHandler) bool
}
