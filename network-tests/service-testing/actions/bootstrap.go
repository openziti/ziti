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

	// Setup Ziti Controller
	workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/*"))
	workflow.AddAction(component.Stop("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	//Comment out the below line if you are inserting your own DB.
	workflow.AddAction(edge.InitController("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(component.Start("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Login to Ziti Controller
	workflow.AddAction(edge.Login("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Setup Ziti Routers
	workflow.AddAction(component.StopInParallel(models.EdgeRouterTag, 25))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(edge.InitEdgeRouters(models.EdgeRouterTag, 2))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(component.StartInParallel(models.EdgeRouterTag, 25))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Init Identities
	workflow.AddAction(edge.InitIdentities(models.SdkAppTag, 2))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Create Configs
	workflow.AddAction(zitilib_actions.Edge("create", "config", "iperf-server", "host.v1", `
					{
							"address" : "localhost",
							"port" : 7001,
							"protocol" : "tcp"
					}`))
	workflow.AddAction(semaphore.Sleep(120 * time.Second))
	workflow.AddAction(zitilib_actions.Edge("create", "config", "iperf-intercept", "intercept.v1", `
		{
			"addresses": ["iperf.service"],
			"portRanges" : [
				{ "low": 7001, "high": 7001 }
			 ],
			"protocols": ["tcp"]
		}`))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Start Beats Services
	workflow.AddAction(host.GroupExec("*", 25, "sudo service filebeat stop; sleep 5; sudo service filebeat start"))
	workflow.AddAction(host.GroupExec("*", 25, "sudo service metricbeat stop; sleep 5; sudo service metricbeat start"))

	return workflow
}
