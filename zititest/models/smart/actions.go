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

package main

import (
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/models/smart/actions"
	zitilab_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/console"
)

func newActionsFactory() model.Factory {
	return &actionsFactory{}
}

func (_ *actionsFactory) Build(m *model.Model) error {
	m.Actions = model.ActionBinders{
		"bootstrap": actions.NewBootstrapAction(),
		"start":     actions.NewStartAction(),
		"stop":      actions.NewStopAction(),
		"console":   func(m *model.Model) model.Action { return console.Console() },
		"logs":      func(m *model.Model) model.Action { return zitilab_actions.Logs() },
	}
	return nil
}

type actionsFactory struct{}
