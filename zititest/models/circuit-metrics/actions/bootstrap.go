package actions

import (
	"fmt"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/ziti/zititest/zitilab"
	"time"

	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
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

	//Start Ziti Controller
	workflow.AddAction(host.GroupExec("#ctrl", 1, "rm -f logs/*"))
	workflow.AddAction(component.Stop("#ctrl"))
	workflow.AddAction(component.Exec("#ctrl", zitilab.ControllerActionInitStandalone))
	workflow.AddAction(component.Start("#ctrl"))
	workflow.AddAction(edge.ControllerAvailable("#ctrl", 30*time.Second))

	// Login to Ziti Controller
	workflow.AddAction(edge.Login("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Setup Ziti Routers
	workflow.AddAction(component.StopInParallel(models.EdgeRouterTag, 25))
	fmt.Print("Starting Edge Routers")
	workflow.AddAction(component.StartInParallel(models.EdgeRouterTag, 25))
	//fmt.Print("Init Edge Routers")
	//workflow.AddAction(edge.InitEdgeRouters(models.EdgeRouterTag, 2))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Init Identities
	fmt.Print("Init Identities")
	workflow.AddAction(edge.InitIdentities(models.SdkAppTag, 2))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Create Configs
	workflow.AddAction(zitilib_actions.Edge("create", "config", "iperf-server", "host.v1", `
					{
							"address" : "localhost",
							"port" : 7001,
							"protocol" : "tcp"
					}`))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(zitilib_actions.Edge("create", "config", "iperf-intercept", "intercept.v1", `
		{
			"addresses": ["iperf.service"],
			"portRanges" : [
				{ "low": 7001, "high": 7001 }
			 ],
			"protocols": ["tcp"]
		}`))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "client-routers", "--edge-router-roles", "#client", "--identity-roles", "#client"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "host-routers", "--edge-router-roles", "#host", "--identity-roles", "#host"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	//workflow.AddAction(component.Stop(models.ControllerTag))
	workflow.AddAction(host.GroupExec("ctrl", 25, "sudo service filebeat stop; sleep 5; sudo service filebeat start"))
	workflow.AddAction(host.GroupExec("ctrl", 25, "sudo service metricbeat stop; sleep 5; sudo service metricbeat start"))
	return workflow
}
