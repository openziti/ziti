/*
	Copyright 2019 NetFoundry Inc.

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

package zitilib_actions

import (
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/cli"
)

func Fabric(args ...string) model.Action {
	return &fabric{
		args: args,
	}
}

func (a *fabric) Execute(run model.Run) error {
	_, err := cli.Exec(run.GetModel(), append([]string{"fabric", "-i", model.ActiveInstanceId()}, a.args...)...)
	return err
}

type fabric struct {
	args []string
}
