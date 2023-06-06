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
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"time"
)

func NewBootstrapAction() model.ActionBinder {
	action := &bootstrapAction{}
	return action.bind
}

func (a *bootstrapAction) bind(m *model.Model) model.Action {
	workflow := actions.Workflow()

	workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/*"))
	workflow.AddAction(component.Stop("#ctrl"))
	workflow.AddAction(edge.InitController("#ctrl"))
	workflow.AddAction(component.Start("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	workflow.AddAction(edge.Login("#ctrl"))

	workflow.AddAction(component.StopInParallel(models.EdgeRouterTag, 25))
	workflow.AddAction(edge.InitEdgeRouters(models.EdgeRouterTag, 2))
	workflow.AddAction(edge.InitIdentities(models.SdkAppTag, 2))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "test-host", "host.v2", `
		{
			"terminators" : [ 
					{
							"address" : "localhost",
							"port" : 8171,
							"protocol" : "tcp",
							"portChecks" : [
								{
									 "address" : "localhost:8172",
									 "interval" : "1s",
									 "timeout" : "100ms",
									 "actions" : [
										 { "trigger" : "fail", "action" : "mark unhealthy" },
										 { "trigger" : "pass", "action" : "mark healthy" }
									 ]
								}
						   ]
					}
			]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "test-intercept", "intercept.v1", `
		{
			"addresses": ["test.service"],
			"portRanges" : [ 
				{ "low": 8171, "high": 8171 } 
			 ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "service", "test", "--encryption", "on", "-c", "test-host,test-intercept"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "metrics", "--encryption", "on"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "test-bind", "Bind", "--service-roles", "@test", "--identity-roles", "#terminator"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "test-dial", "Dial", "--service-roles", "@test", "--identity-roles", "#initiator,#client", "--semantic", "AnyOf"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "metrics-dial", "Dial", "--service-roles", "@metrics", "--identity-roles", "#client"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "metrics-bind", "Bind", "--service-roles", "@metrics", "--identity-roles", "#metrics-host"))

	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "metrics-routers", "--edge-router-roles", "@metrics-router", "--identity-roles", "#all"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "test-routers", "--edge-router-roles", "@router-ap-0", "--identity-roles", "#client"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "test", "--semantic", "AnyOf", "--service-roles", "@test", "--edge-router-roles", "#initiator,#terminator"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "metrics", "--service-roles", "@metrics", "--edge-router-roles", "@metrics-router"))

	workflow.AddAction(component.Stop(models.ControllerTag))

	return workflow
}

type bootstrapAction struct{}
