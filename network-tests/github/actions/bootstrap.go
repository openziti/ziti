package actions

import (
	"time"

	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/zitilab/actions"
	"github.com/openziti/zitilab/actions/edge"
	"github.com/openziti/zitilab/models"
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

	workflow.AddAction(zitilib_actions.Edge("create", "service", "test", "--encryption", "on", "-c", "test-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "metrics", "--encryption", "on"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "metrics-dial", "Dial", "--service-roles", "@metrics", "--identity-roles", "#client"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "metrics-bind", "Bind", "--service-roles", "@metrics", "--identity-roles", "#metrics-host"))

	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "metrics-routers", "--edge-router-roles", "@metrics-router", "--identity-roles", "#all"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "test-routers", "--edge-router-roles", "@router-west-0", "--identity-roles", "#client"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "test", "--semantic", "AnyOf", "--service-roles", "@test", "--edge-router-roles", "#initiator,#terminator"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "metrics", "--service-roles", "@metrics", "--edge-router-roles", "@metrics-router"))

	workflow.AddAction(component.Stop(models.ControllerTag))

	return workflow
}

type bootstrapAction struct{}
