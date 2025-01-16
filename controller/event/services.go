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

const (
	ServiceEventNS = "service"
)

// A ServiceEvent is emitted for service and terminator level metrics which are collected per some interval.
//
// Note: In version prior to 1.4.0, the config key was `services`, but the namespace was `service.events`
//
// Value values for the service event type are:
//   - service.dial.success
//   - service.dial.fail
//   - service.dial.timeout
//   - service.dial.error_other
//   - service.dial.terminator.timeout
//   - service.dial.terminator.connection_refused
//   - service.dial.terminator.invalid
//   - service.dial.terminator.misconfigured
//
// Example: Dial Success for a specific service and terminator
//
//	{
//	 "namespace": "service",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2024-10-02T12:17:40.501821249-04:00"
//	 "version": 2,
//	 "event_type": "service.dial.success",
//	 "terminator_id": "2xFBuwwzJzAXuw5lOPnDwr",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP",
//	 "count": 1,
//	 "interval_start_utc": 1737140460,
//	 "interval_length": 60
//	}
//
// Example: Dail failures or type 'Other' for a specific service
//
//	{
//	 "namespace": "service",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2024-10-02T12:17:40.501821249-04:00"
//	 "version": 2,
//	 "event_type": "service.dial.error_other",
//	 "terminator_id": "",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP",
//	 "count": 1,
//	 "interval_start_utc": 1737140580,
//	 "interval_length": 60
//	}
type ServiceEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The event version. The current version is 2.
	Version uint32 `json:"version"`

	// The services event type. See above for valid values.
	EventType string `json:"event_type"`

	// The terminator id, if this is representing a terminator specific metric.
	TerminatorId string `json:"terminator_id"`

	// The service identifier.
	ServiceId string `json:"service_id"`

	// The number of events that have happened in the given time interval
	Count uint64 `json:"count"`

	// The start time of the interval. It is represented as Unix time, number of seconds
	// since the beginning of the current epoch.
	IntervalStartUTC int64 `json:"interval_start_utc"`

	// The interval length in seconds.
	IntervalLength uint64 `json:"interval_length"`
}

func (event *ServiceEvent) String() string {
	return fmt.Sprintf("%v service=%v terminator=%v count=%v intervalStart=%v intervalLength=%v",
		event.EventType, event.ServiceId, event.TerminatorId, event.Count, event.IntervalStartUTC, event.IntervalLength)
}

type ServiceEventHandler interface {
	AcceptServiceEvent(event *ServiceEvent)
}

type ServiceEventHandlerWrapper interface {
	ServiceEventHandler
	IsWrapping(value ServiceEventHandler) bool
}
