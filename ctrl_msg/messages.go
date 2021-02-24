/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/foundation/channel2"
)

const (
	SessionSuccessType      = 1001
	SessionFailedType       = 1016
	RouteResultType         = 1022
	SessionConfirmationType = 1034

	SessionSuccessAddressHeader = 1100
	RouteResultAttemptHeader    = 1101
	RouteResultSuccessHeader    = 1102
	RouteResultErrorHeader      = 1103
)

func NewSessionSuccessMsg(sessionId, address string) *channel2.Message {
	msg := channel2.NewMessage(SessionSuccessType, []byte(sessionId))
	msg.Headers[SessionSuccessAddressHeader] = []byte(address)
	return msg
}

func NewSessionFailedMsg(message string) *channel2.Message {
	return channel2.NewMessage(SessionFailedType, []byte(message))
}

func NewRouteResultSuccessMsg(sessionId string, attempt int) *channel2.Message {
	msg := channel2.NewMessage(RouteResultType, []byte(sessionId))
	msg.PutUint32Header(RouteResultAttemptHeader, uint32(attempt))
	msg.PutUint32Header(RouteResultAttemptHeader, uint32(attempt))
	msg.PutBoolHeader(RouteResultSuccessHeader, true)
	return msg
}

func NewRouteResultFailedMessage(sessionId string, attempt int, rerr string) *channel2.Message {
	msg := channel2.NewMessage(RouteResultType, []byte(sessionId))
	msg.PutUint32Header(RouteResultAttemptHeader, uint32(attempt))
	msg.Headers[RouteResultErrorHeader] = []byte(rerr)
	return msg
}
