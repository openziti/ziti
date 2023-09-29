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

const SessionEventTypeCreated = "created"
const SessionEventTypeDeleted = "deleted"
const SessionEventNS = "edge.sessions"

type SessionEvent struct {
	Namespace    string    `json:"namespace"`
	EventType    string    `json:"event_type"`
	SessionType  string    `json:"session_type"`
	Id           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Token        string    `json:"token"`
	ApiSessionId string    `json:"api_session_id"`
	IdentityId   string    `json:"identity_id"`
	ServiceId    string    `json:"service_id"`
}

func (event *SessionEvent) String() string {
	return fmt.Sprintf("%v.%v id=%v timestamp=%v token=%v apiSessionId=%v identityId=%v",
		event.Namespace, event.EventType, event.Id, event.Timestamp, event.Token, event.ApiSessionId, event.IdentityId)
}

type SessionEventHandler interface {
	AcceptSessionEvent(event *SessionEvent)
}

type SessionEventHandlerWrapper interface {
	SessionEventHandler
	IsWrapping(value SessionEventHandler) bool
}
