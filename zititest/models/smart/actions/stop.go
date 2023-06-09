/*
	Copyright 2020 NetFoundry Inc.

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
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/models"
)

func NewStopAction() model.ActionBinder {
	action := &stopAction{}
	return action.bind
}

func (_ *stopAction) bind(m *model.Model) model.Action {
	return actions.Workflow(
		host.GroupKill(models.LoopDialerTag, "ziti-fabric-test"),
		host.GroupKill(models.LoopListenerTag, "ziti-fabric-test"),
		component.Stop(models.RouterTag),
		component.Stop(models.ControllerTag),
	)
}

type stopAction struct{}
