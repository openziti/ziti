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

import (
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/cmd/ziti/tutorial"
	"net/url"
)

type ZitiEnsureLoggedIn struct {
	LoginParams LoginParams
}

func (self *ZitiEnsureLoggedIn) Execute(ctx *tutorial.ActionContext) error {
	_, _, err := edge.ListEntitiesOfType("edge-routers", url.Values{}, false, nil, 10, false)
	if err == nil {
		fmt.Println("Already authenticated to Ziti Controller")
		return nil
	}
	loginAction := &ZitiLoginAction{LoginParams: self.LoginParams}
	return loginAction.Execute(ctx)
}
