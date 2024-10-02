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
	SdkEventsNs = "sdk"

	SdkOnline        SdkEventType = "sdk-online"
	SdkOffline       SdkEventType = "sdk-offline"
	SdkStatusUnknown SdkEventType = "sdk-status-unknown"
)

type SdkEvent struct {
	Namespace  string       `json:"namespace"`
	EventType  SdkEventType `json:"event_type"`
	Timestamp  time.Time    `json:"timestamp"`
	IdentityId string       `json:"identity_id"`
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
