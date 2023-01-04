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
	CircuitEventsNs                       = "fabric.circuits"
	CircuitEventsVersion                  = 2
	CircuitCreated       CircuitEventType = "created"
	CircuitUpdated       CircuitEventType = "pathUpdated"
	CircuitDeleted       CircuitEventType = "deleted"
	CircuitFailed        CircuitEventType = "failed"
)

var CircuitEventTypes = []CircuitEventType{CircuitCreated, CircuitUpdated, CircuitDeleted, CircuitFailed}

type CircuitPath struct {
	Nodes                []string `json:"nodes"`
	Links                []string `json:"links"`
	IngressId            string   `json:"ingress_id"`
	EgressId             string   `json:"egress_id"`
	TerminatorLocalAddr  string   `json:"terminator_local_addr"`
	TerminatorRemoteAddr string   `json:"terminator_remote_addr"`
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

type CircuitEvent struct {
	Namespace        string           `json:"namespace"`
	Version          uint32           `json:"version"`
	EventType        CircuitEventType `json:"event_type"`
	CircuitId        string           `json:"circuit_id"`
	Timestamp        time.Time        `json:"timestamp"`
	ClientId         string           `json:"client_id"`
	ServiceId        string           `json:"service_id"`
	TerminatorId     string           `json:"terminator_id"`
	InstanceId       string           `json:"instance_id"`
	CreationTimespan *time.Duration   `json:"creation_timespan,omitempty"`
	Path             CircuitPath      `json:"path"`
	LinkCount        int              `json:"link_count"`
	Cost             *uint32          `json:"path_cost,omitempty"`
	FailureCause     *string          `json:"failure_cause,omitempty"`
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
