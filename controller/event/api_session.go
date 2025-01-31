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

const ApiSessionEventTypeCreated = "created"
const ApiSessionEventTypeDeleted = "deleted"
const ApiSessionEventTypeRefreshed = "refreshed"
const ApiSessionEventTypeExchanged = "exchanged"
const ApiSessionEventNS = "apiSession"

const ApiSessionTypeLegacy = "legacy"
const ApiSessionTypeJwt = "jwt"

// An ApiSessionEvent is emitted whenever an api session is created, deleted, refreshed or exchanged.
// Legacy sessions are only ever created or deleted. JWT sessions are created, refreshed and exchanged.
//
// Note: In version prior to 1.4.0, the namespace was `edge.apiSessions`
//
// Valid api session event types are:
//   - created
//   - deleted
//   - refreshed
//   - exchanged
//
// Valid api session types are:
//   - jwt
//   - legacy
//
// Example: Api Session Created Event
//
//	{
//	    "namespace": "apiSession",
//		"event_src_id" : "ctrl1",
//		"timestamp": "2021-11-08T14:45:45.785561479-05:00",
//		"event_type": "created",
//		"id": "ckvr2r4fs0001oigd6si4akc8",
//		"token": "77cffde5-f68e-4ef0-bbb5-731db36145f5",
//		"identity_id": "76BB.shC0",
//		"ip_address": "127.0.0.1"
//	}
type ApiSessionEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The type api session event. See above for valid values.
	EventType string `json:"event_type"`

	// Id is the api session id.
	Id string `json:"id"`

	// Type is the api session type. See above for valid values.
	Type string `json:"type"`

	// The api session token.
	Token string `json:"token"`

	// The id of the identity that the api session belongs to.
	IdentityId string `json:"identity_id"`

	// The IP address from which the identity to connected to require the api session.
	IpAddress string `json:"ip_address"`
}

func (event *ApiSessionEvent) String() string {
	return fmt.Sprintf("%v.%v id=%v timestamp=%v token=%v identityId=%v ipAddress=%v",
		event.Namespace, event.EventType, event.Id, event.Timestamp, event.Token, event.IdentityId, event.IpAddress)
}

type ApiSessionEventHandler interface {
	AcceptApiSessionEvent(event *ApiSessionEvent)
}

type ApiSessionEventHandlerWrapper interface {
	ApiSessionEventHandler
	IsWrapping(value ApiSessionEventHandler) bool
}
