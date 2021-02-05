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
	ContentTypeSessionSuccessType = 1001
	ContentTypeSessionFailedType  = 1016
	RouteResultType               = 1022

	SessionSuccessAddressHeader = 1100
	RouteResultSuccessHeader    = 1101
	RouteResultErrorHeader      = 1102
)

func NewSessionSuccessMsg(sessionId, address string) *channel2.Message {
	msg := channel2.NewMessage(ContentTypeSessionSuccessType, []byte(sessionId))
	msg.Headers[SessionSuccessAddressHeader] = []byte(address)
	return msg
}

func NewSessionFailedMsg(message string) *channel2.Message {
	return channel2.NewMessage(ContentTypeSessionFailedType, []byte(message))
}

func NewRouteResultSuccessMsg(sessionId string) *channel2.Message {
	msg := channel2.NewMessage(RouteResultType, []byte(sessionId))
	msg.Headers[RouteResultSuccessHeader] = []byte{1}
	return msg
}

func NewRouteResultFailedMessage(sessionId, err string) *channel2.Message {
	msg := channel2.NewMessage(RouteResultType, []byte(sessionId))
	msg.Headers[RouteResultErrorHeader] = []byte(err)
	return msg
}
