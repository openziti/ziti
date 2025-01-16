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

type SdkEventType string

const (
	SdkEventNS = "sdk"

	SdkOnline        SdkEventType = "sdk-online"
	SdkOffline       SdkEventType = "sdk-offline"
	SdkStatusUnknown SdkEventType = "sdk-status-unknown"
)

// An SdkEvent is emitted when an sdk's connectivity to routers changes.
//
// Valid values for sdk event type are:
//   - sdk-online - identity is online
//   - sdk-offline - identity is offline
//   - sdk-status-unknown - status is unknown because the routers that reported the identity online are not connected to the controller
//
// Example: SDK identity is online
//
//	{
//	 "namespace": "sdk",
//	 "event_src_id": "ctrl1",
//	 "event_type" : "sdk-online",
//	 "identity_id": "ji2Rt8KJ4",
//	 "timestamp": "2024-10-02T12:17:39.501821249-04:00"
//	}
//
// Example: SDK identity online status is unknown
//
//	{
//	 "namespace": "sdk",
//	 "event_src_id": "ctrl1",
//	 "event_type" : "sdk-status-unknown",
//	 "identity_id": "ji2Rt8KJ4",
//	 "timestamp": "2024-10-02T12:17:40.501821249-04:00"
//	}
//
// Example: SDK identit is offline
//
//	{
//	 "namespace": "sdk",
//	 "event_src_id": "ctrl1",
//	 "event_type" : "sdk-offline",
//	 "identity_id": "ji2Rt8KJ4",
//	 "timestamp": "2024-10-02T12:17:41.501821249-04:00"
//	}
type SdkEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The sdk event type. See above for valid values.
	EventType SdkEventType `json:"event_type"`

	// The id of the identity whose connectivity state has changed.
	IdentityId string `json:"identity_id"`
}

func (event *SdkEvent) String() string {
	return fmt.Sprintf("%v.%v time=%v identityId=%v",
		event.Namespace, event.EventType, event.Timestamp, event.IdentityId)
}

type SdkEventHandler interface {
	AcceptSdkEvent(event *SdkEvent)
}

type SdkEventHandlerWrapper interface {
	SdkEventHandler
	IsWrapping(value SdkEventHandler) bool
}
