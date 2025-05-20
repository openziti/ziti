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
	AuthenticationEventNS = "authentication"

	AuthenticationEventTypeFail    = "fail"
	AuthenticationEventTypeSuccess = "success"
)

// An AuthenticationEvent is emitted when an authentication attempt is made
//
// Types of authentication events
//   - failed - authentication failed
//   - success - authentication succeeded
//
// Types of authentication methods
//   - updb - username password from the internal database
//   - cert - a certificate, either first party or 3rd party
//   - ext-jwt - an external JWT from an IDP
//
// Example: Authentication Failed Event
//
//	{
//	 "namespace": "authentication",
//	 "event_src_id": "ctrl1",
//	 "timestamp": "2025-05-12T14:30:00Z",
//	 "event_type": "failed",
//	 "type": "updb",
//	 "authenticator_id": "auth01",
//	 "external_jwt_signer_id": "",
//	 "identity_id": "id42",
//	 "auth_policy_id": "pol7",
//	 "ip_address": "192.0.2.10",
//	 "success": false,
//	 "reason": "invalid password",
//	 "improper_client_cert_chain": "false"
//	}
type AuthenticationEvent struct {
	Namespace  string    `json:"namespace"`
	EventSrcId string    `json:"event_src_id"`
	Timestamp  time.Time `json:"timestamp"`

	// The type of the authentication event. See above for valid values.
	EventType string `json:"event_type"`

	// method is the authentication method type. See above for valid values.
	Method string `json:"type"`

	// The id of the authenticator associated with the authentication attempt, may be empty (e.g. for etx-jwt)
	AuthenticatorId string `json:"authenticator_id"`

	// ExternalJwtSignerId is the external jwt signer id triggered, may be empty (e.g. for cert)
	ExternalJwtSignerId string `json:"external_jwt_signer_id"`

	// The id of the identity that the authentication is attempted for, may be blank of no identity is resolved
	IdentityId string `json:"identity_id"`

	// The id of the authentication policy which allowed access, may be blank if no auth policy is resolved (e.g. identity is also blank)
	AuthPolicyId string `json:"auth_policy_id"`

	// The remote address from which the authentication request was issues
	RemoteAddress string `json:"remote_address"`

	// Success is true if the Authentication and associated authentication policies provide access, false otherwise
	Success bool `json:"success"`

	// FailureReason contains the reason the Authentication failed
	FailureReason string `json:"reason"`

	// ImproperClientCertChain is false for all authentication methods other than cert. When true, it indicates
	// a network issued certificate was used during authentication and its chain was not provided or did not map
	// to the network root CA. This indicates enrollment with an older controller or SDK that did not send/save
	// the proper chain.
	ImproperClientCertChain bool `json:"improper_client_cert_chain"`
}

func (event *AuthenticationEvent) String() string {
	return fmt.Sprintf("%v.%v method=%v, authenticatorId=%v timestamp=%v identityId=%v ipAddress=%v extJwtSignerId=%v authPolicyId=%v impropertClientCertChain=%v success=%v reason=%v",
		event.Namespace, event.EventType, event.Method, event.AuthenticatorId, event.Timestamp, event.IdentityId, event.RemoteAddress,
		event.ExternalJwtSignerId, event.AuthPolicyId, event.ImproperClientCertChain, event.Success, event.FailureReason)
}

type AuthenticationEventHandler interface {
	AcceptAuthenticationEvent(event *AuthenticationEvent)
}

type AuthenticationEventHandlerWrapper interface {
	AuthenticationEventHandler
	IsWrapping(value AuthenticationEventHandler) bool
}
