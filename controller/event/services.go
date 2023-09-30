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

import "fmt"

const (
	ServiceEventsNs = "services"
)

type ServiceEvent struct {
	Namespace        string `json:"namespace"`
	Version          uint32 `json:"version"`
	EventType        string `json:"event_type"`
	ServiceId        string `json:"service_id"`
	TerminatorId     string `json:"terminator_id"`
	Count            uint64 `json:"count"`
	IntervalStartUTC int64  `json:"interval_start_utc"`
	IntervalLength   uint64 `json:"interval_length"`
}

func (event *ServiceEvent) String() string {
	return fmt.Sprintf("%v service=%v terminator=%v count=%v intervalStart=%v intervalLength=%v",
		event.EventType, event.ServiceId, event.TerminatorId, event.Count, event.IntervalStartUTC, event.IntervalLength)
}

type ServiceEventHandler interface {
	AcceptServiceEvent(event *ServiceEvent)
}
