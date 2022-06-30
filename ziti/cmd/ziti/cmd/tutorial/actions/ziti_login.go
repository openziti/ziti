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

package actions

import "github.com/openziti/ziti/ziti/cmd/ziti/tutorial"

type LoginParams interface {
	GetControllerUrl() string
	GetUsername() string
	GetPassword() string
}

type ZitiLoginAction struct {
	LoginParams LoginParams
}

func (self *ZitiLoginAction) Execute(ctx *tutorial.ActionContext) error {
	cmd := "ziti edge login --ignore-config"
	if self.LoginParams.GetControllerUrl() != "" {
		cmd += " " + self.LoginParams.GetControllerUrl()
	}
	if self.LoginParams.GetUsername() != "" {
		cmd += " --username " + self.LoginParams.GetUsername()
	}
	if self.LoginParams.GetPassword() != "" {
		cmd += " --password " + self.LoginParams.GetPassword()
	}
	ctx.Body = cmd
	return (&ZitiRunnerAction{}).Execute(ctx)
}
