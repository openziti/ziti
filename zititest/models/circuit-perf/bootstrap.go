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
	"github.com/openziti/ziti/zititest/zitilab"
	"time"

	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
)

type bootstrapAction struct{}

func NewBootstrapAction() model.ActionBinder {
	action := &bootstrapAction{}
	return action.bind
}

func (a *bootstrapAction) bind(m *model.Model) model.Action {
	workflow := actions.Workflow()

	workflow.AddAction(component.Stop(".ctrl"))
	workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/*"))
	workflow.AddAction(host.GroupExec("component.ctrl", 5, "rm -rf ./fablab/ctrldata"))

	workflow.AddAction(component.Exec("#ctrl", zitilab.ControllerActionInitStandalone))
	workflow.AddAction(component.Start(".ctrl"))
	workflow.AddAction(edge.ControllerAvailable("#ctrl", 30*time.Second))
	workflow.AddAction(edge.Login("#ctrl"))

	workflow.AddAction(component.StopInParallel(models.EdgeRouterTag, 25))
	workflow.AddAction(edge.InitEdgeRouters(models.EdgeRouterTag, 2))
	workflow.AddAction(edge.InitIdentities(models.SdkAppTag, 2))

	// SSH service
	workflow.AddAction(zitilib_actions.Edge("create", "config", "ssh-host", "host.v1", `
		{
			"address" : "localhost",
			"port" : 22,
			"protocol" : "tcp"
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "ssh-intercept", "intercept.v1", `
		{
			"addresses": ["ssh.ziti"],
			"portRanges" : [ { "low": 2022, "high": 2022 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "service", "ssh", "-c", "ssh-intercept,ssh-host"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "ssh-hosts", "Bind", "--service-roles", "@ssh", "--identity-roles", "#ssh-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "ssh-clients", "Dial", "--service-roles", "@ssh", "--identity-roles", "#ssh-client"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "ssh", "--service-roles", "@ssh", "--edge-router-roles", "#test"))

	// Loop Service
	workflow.AddAction(zitilib_actions.Edge("create", "config", "loop-host", "host.v1", `
		{
			"address" : "localhost",
			"port" : 3456,
			"protocol" : "tcp"
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "service", "throughput", "-c", "loop-host", "-a", "loop"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "latency", "-c", "loop-host", "-a", "loop"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "slow", "-c", "loop-host", "-a", "loop"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-hosts", "Bind", "--service-roles", "#loop", "--identity-roles", "#loop-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-clients", "Dial", "--service-roles", "#loop", "--identity-roles", "#loop-client"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "loop", "--service-roles", "#loop", "--edge-router-roles", "#test"))

	// Metrics Service
	workflow.AddAction(zitilib_actions.Edge("create", "service", "metrics"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "metrics-hosts", "Bind", "--service-roles", "@metrics", "--identity-roles", "#metrics-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "metrics-clients", "Dial", "--service-roles", "@metrics", "--identity-roles", "#metrics-client"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "metrics-hosts", "--edge-router-roles", "#metrics", "--identity-roles", "#metrics-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "metrics-clients", "--edge-router-roles", "#metrics", "--identity-roles", "#metrics-client"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "metrics", "--service-roles", "@metrics", "--edge-router-roles", "#metrics"))

	// Shared policies
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "hosts", "--edge-router-roles", "#host", "--identity-roles", "#host"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "clients", "--edge-router-roles", "#client", "--identity-roles", "#client"))

	workflow.AddAction(component.Stop(models.ControllerTag))

	return workflow
}
