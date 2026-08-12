/*
	(c) Copyright NetFoundry Inc.

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
	"time"

	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
)

type startAction struct{}

// newStartAction starts the phase-1 system: the single initial controller, the routers, the
// loop4 hosts/backends, and finally the clients (including the remote-controlled sim client).
func newStartAction() model.ActionBinder {
	action := &startAction{}
	return action.bind
}

func (a *startAction) bind(m *model.Model) model.Action {
	workflow := actions.Workflow()

	workflow.AddAction(component.Start("#ctrl1"))
	workflow.AddAction(edge.ControllerAvailable("#ctrl1", 30*time.Second))
	workflow.AddAction(edge.Login("#ctrl1"))

	workflow.AddAction(component.StartInParallel(models.EdgeRouterTag, 10))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// hosts: plain-TCP loop4 backends, the SDK loop4 host, and the ZET/ziti-tunnel hosts
	workflow.AddAction(component.StartInParallel(".loop-backend", 10))
	workflow.AddAction(component.StartInParallel(".sdk-app.host", 10))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// clients: ZET client, ziti-tunnel client, and the remote-controlled sim client
	workflow.AddAction(component.StartInParallel(".sdk-app.client", 10))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	return workflow
}
