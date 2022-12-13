package actions

import (
	zitilib_actions "github.com/openziti/zitilab/actions"
	"time"

	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/zitilab/actions/edge"
	"github.com/openziti/zitilab/models"
)

type bootstrapAction struct{}

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

	//workflow.AddAction(zitilib_actions.Edge("create", "service", "echo"))
	workflow.AddAction(zitilib_actions.Edge("create", "config", "iperf-server", "host.v1", `		 
					{
							"address" : "localhost",
							"port" : 7001,
							"protocol" : "tcp"
					}`))
	workflow.AddAction(zitilib_actions.Edge("create", "config", "iperf-intercept", "intercept.v1", `
		{
			"addresses": ["iperf.service"],
			"portRanges" : [ 
				{ "low": 7001, "high": 7001 } 
			 ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "service", "iperf", "-c", "iperf-server,iperf-intercept"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "iperf-server", "Bind", "--service-roles",
		"@iperf", "--identity-roles", "#iperf-server"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "iperf-client", "Dial", "--service-roles",
		"@iperf", "--identity-roles", "#iperf-client"))

	//
	//workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "iperf-server", "--edge-router-roles",
	//"@router-east-server", "--identity-roles", "#server"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "iperf-client", "--edge-router-roles",
		"#iperf-client", "--identity-roles", "#iperf-client"))
	//workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "iperf-client", "--edge-router-roles",
	//"@router-tokyo-client", "--identity-roles", "#client"))
	//workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "iperf-client", "--edge-router-roles",
	//"@router-sydney-client", "--identity-roles", "#client"))
	//workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "iperf-client", "--edge-router-roles",
	//"@router-canada-client", "--identity-roles", "#client"))
	//workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "iperf-client", "--edge-router-roles",
	//"@router-frankfurt-client", "--identity-roles", "#client"))
	//workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "iperf-client", "--edge-router-roles",
	//"@router-brazil-client", "--identity-roles", "#client"))
	//workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "iperf-client", "--edge-router-roles",
	//"@router-east-client", "--identity-roles", "#client"))
	//workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "iperf-client", "--edge-router-roles",
	//"@router-west-client", "--identity-roles", "#client"))

	//workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "echo", "--semantic", "AnyOf",
	//"--service-roles", "@echo", "--edge-router-roles", "#all"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "iperf", "--semantic", "AnyOf",
		"--service-roles", "@iperf", "--edge-router-roles", "#all"))
	//workflow.AddAction(component.Stop(models.ControllerTag))

	return workflow
}
