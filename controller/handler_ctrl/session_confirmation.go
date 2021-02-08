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

package handler_ctrl

import (
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/foundation/channel2"
	"github.com/sirupsen/logrus"
)

type sessionConfirmationHandler struct{}

func newSessionConfirmationHandler() *sessionConfirmationHandler {
	return &sessionConfirmationHandler{}
}

func (self *sessionConfirmationHandler) ContentType() int32 {
	return int32(ctrl_msg.SessionConfirmationType)
}

func (self *sessionConfirmationHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	logrus.Infof("this controller does not process session confirmation broadcasts. consider upgrading to a newer controller")
}