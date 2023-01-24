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

package ctrl_msg

import (
	"github.com/openziti/channel/v2"
)

const (
	CircuitSuccessType = 1001
	CircuitFailedType  = 1016
	RouteResultType    = 1022

	CircuitSuccessAddressHeader = 1100
	RouteResultAttemptHeader    = 1101
	RouteResultSuccessHeader    = 1102
	RouteResultErrorHeader      = 1103
	RouteResultErrorCodeHeader  = 1104

	TerminatorLocalAddressHeader  = 1110
	TerminatorRemoteAddressHeader = 1111

	InitiatorLocalAddressHeader  = 1112
	InitiatorRemoteAddressHeader = 1113

	ErrorTypeGeneric                 = 0
	ErrorTypeInvalidTerminator       = 1
	ErrorTypeMisconfiguredTerminator = 2
	ErrorTypeDialTimedOut            = 3
	ErrorTypeConnectionRefused       = 4
)

func NewCircuitSuccessMsg(sessionId, address string) *channel.Message {
	msg := channel.NewMessage(CircuitSuccessType, []byte(sessionId))
	msg.Headers[CircuitSuccessAddressHeader] = []byte(address)
	return msg
}

func NewCircuitFailedMsg(message string) *channel.Message {
	return channel.NewMessage(CircuitFailedType, []byte(message))
}

func NewRouteResultSuccessMsg(sessionId string, attempt int) *channel.Message {
	msg := channel.NewMessage(RouteResultType, []byte(sessionId))
	msg.PutUint32Header(RouteResultAttemptHeader, uint32(attempt))
	msg.PutUint32Header(RouteResultAttemptHeader, uint32(attempt))
	msg.PutBoolHeader(RouteResultSuccessHeader, true)
	return msg
}

func NewRouteResultFailedMessage(sessionId string, attempt int, rerr string) *channel.Message {
	msg := channel.NewMessage(RouteResultType, []byte(sessionId))
	msg.PutUint32Header(RouteResultAttemptHeader, uint32(attempt))
	msg.Headers[RouteResultErrorHeader] = []byte(rerr)
	return msg
}
