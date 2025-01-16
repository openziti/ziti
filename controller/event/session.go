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

const SessionEventNS = "session"

const SessionEventTypeCreated = "created"
const SessionEventTypeDeleted = "deleted"

// A SessionEvent is emitted when a session is created or deleted.
//
// Note: In version prior to 1.4.0, the namespace was `edge.sessions`
//
// Valid values for session type are:
//   - created
//   - deleted
//
// Example: Bind Session created event
//
//	{
//	 "namespace": "session",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:29:53.204988284-05:00",
//	 "event_type": "created",
//	 "session_type": "Bind",
//	 "id": "cm611bn75000jdhj9s5xrwynr",
//	 "token": "4ed77b84-650e-4b4b-9fbb-2466c9c94abb",
//	 "api_session_id": "cm611bn6l000hdhj9urp9xlnw",
//	 "identity_id": "IahyE.5Scw",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP"
//	}
//
// Example: Bind session deleted event
//
//	{
//	 "namespace": "session",
//	 "event_src_id": "ctrl_client",
//	 "timestamp": "2025-01-17T12:30:08.593650693-05:00",
//	 "event_type": "deleted",
//	 "session_type": "Bind",
//	 "id": "cm611bgxa0008dhj9k3yo3vox",
//	 "token": "f8a447a2-2bd0-4821-8142-27c1770d06ab",
//	 "api_session_id": "cm611bgwn0006dhj9m4cv1obf",
//	 "identity_id": "IahyE.5Scw",
//	 "service_id": "3pjMOKY2icS8fkQ1lfHmrP"
//	}
type SessionEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The type of session event. See above for valid values.
	EventType string `json:"event_type"`

	// The session type (dial or bind).
	SessionType string `json:"session_type"`

	// The session identifier.
	Id string `json:"id"`

	// The session token.
	Token string `json:"token"`

	// The id of the api session used to create this session.
	ApiSessionId string `json:"api_session_id"`

	// The if of the identity on whose behalf the session was created.
	IdentityId string `json:"identity_id"`

	// The id of the service that this session is claiming access to.
	ServiceId string `json:"service_id"`
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
