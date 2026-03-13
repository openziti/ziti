package main

import (
	"time"

	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
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

	workflow.AddAction(component.StopInParallel("*", 300))
	workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/* ctrl.db"))

	workflow.AddAction(component.Exec("#ctrl1", zitilab.ControllerActionInitStandalone))
	workflow.AddAction(component.Start(".ctrl"))
	workflow.AddAction(edge.ControllerAvailable("#ctrl1", 30*time.Second))
	workflow.AddAction(edge.Login("#ctrl1"))

	workflow.AddAction(component.StopInParallel(models.EdgeRouterTag, 25))
	workflow.AddAction(edge.InitEdgeRouters(models.EdgeRouterTag, 2))
	workflow.AddAction(edge.InitIdentities(models.SdkAppTag, 2))

	// Host config for ERT services
	workflow.AddAction(zitilib_actions.Edge("create", "config", "loop-host", "host.v1", `
		{
			"address" : "localhost",
			"port" : 3456,
			"protocol" : "tcp"
		}`))

	// Intercept configs
	workflow.AddAction(zitilib_actions.Edge("create", "config", "throughput-xg-intercept", "intercept.v1", `
		{
			"addresses": ["throughput-xg.ziti"],
			"portRanges" : [ { "low": 3456, "high": 3456 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "latency-xg-intercept", "intercept.v1", `
		{
			"addresses": ["latency-xg.ziti"],
			"portRanges" : [ { "low": 3456, "high": 3456 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "throughput-ert-intercept", "intercept.v1", `
		{
			"addresses": ["throughput-ert.ziti"],
			"portRanges" : [ { "low": 3456, "high": 3456 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "latency-ert-intercept", "intercept.v1", `
		{
			"addresses": ["latency-ert.ziti"],
			"portRanges" : [ { "low": 3456, "high": 3456 } ],
			"protocols": ["tcp"]
		}`))

	// Services: throughput + latency for xg and ert (no sdk-non-xg, no slow)
	workflow.AddAction(zitilib_actions.Edge("create", "service", "throughput-xg", "-a", "loop,loop-host-xg", "-c", "throughput-xg-intercept"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "latency-xg", "-a", "loop,loop-host-xg", "-c", "latency-xg-intercept"))

	workflow.AddAction(zitilib_actions.Edge("create", "service", "throughput-ert", "-a", "loop,loop-host-ert", "-c", "loop-host,throughput-ert-intercept"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "latency-ert", "-a", "loop,loop-host-ert", "-c", "loop-host,latency-ert-intercept"))

	// Service policies
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-hosts-xg", "Bind", "--service-roles", "#loop-host-xg", "--identity-roles", "#loop-host-xg"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-hosts-ert", "Bind", "--service-roles", "#loop-host-ert", "--identity-roles", "#loop-host-ert"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-clients", "Dial", "--service-roles", "#loop", "--identity-roles", "#loop-client"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "loop", "--service-roles", "#loop", "--edge-router-roles", "#test"))

	// Sim services
	workflow.AddAction(zitilib_actions.Edge("create", "service", "metrics", "-a", "sim-services"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "sim-control", "-a", "sim-services"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "sim-service-hosts", "Bind", "--service-roles", "#sim-services", "--identity-roles", "#sim-services-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "sim-service-clients", "Dial", "--service-roles", "#sim-services", "--identity-roles", "#sim-services-client"))

	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "sim-services-hosts", "--edge-router-roles", "#sim-services", "--identity-roles", "#all"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "sim-services", "--service-roles", "#sim-services", "--edge-router-roles", "#sim-services"))

	// Shared policies
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "hosts", "--edge-router-roles", "#host", "--identity-roles", "#host"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "clients", "--edge-router-roles", "#client", "--identity-roles", "#client"))

	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	workflow.AddAction(component.StartInParallel(models.EdgeRouterTag, 10))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(component.StartInParallel(".sim-services-host", 50))
	workflow.AddAction(component.StartInParallel(".sim-services-client", 50))

	return workflow
}
