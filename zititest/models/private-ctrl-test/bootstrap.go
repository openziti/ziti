package main

import (
	"time"

	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
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

	workflow.AddAction(component.StopInParallel("*", 300))
	workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/* ctrl.db"))
	workflow.AddAction(host.GroupExec("component.ctrl", 5, "rm -rf ./fablab/ctrldata"))

	workflow.AddAction(component.Start(".bootstrap.ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(edge.InitRaftController(".bootstrap.ctrl"))

	workflow.AddAction(edge.ControllerAvailable(".bootstrap.ctrl", 30*time.Second))
	workflow.AddAction(edge.Login(".bootstrap.ctrl"))

	workflow.AddAction(component.StopInParallel(models.RouterTag, 25))
	workflow.AddAction(edge.InitIdentities(models.SdkAppTag, 2))

	// Loop Service configs
	workflow.AddAction(zitilib_actions.Edge("create", "config", "loop-host", "host.v1", `
		{
			"address" : "localhost",
			"port" : 3456,
			"protocol" : "tcp"
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "throughput-intercept", "intercept.v1", `
		{
			"addresses": ["throughput.ziti"],
			"portRanges" : [ { "low": 3456, "high": 3456 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "latency-intercept", "intercept.v1", `
		{
			"addresses": ["latency.ziti"],
			"portRanges" : [ { "low": 3456, "high": 3456 } ],
			"protocols": ["tcp"]
		}`))

	// Services — split by region so traffic must cross routers
	workflow.AddAction(zitilib_actions.Edge("create", "service", "throughput-east", "-a", "loop-east", "-c", "throughput-intercept"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "latency-east", "-a", "loop-east", "-c", "latency-intercept"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "throughput-west", "-a", "loop-west", "-c", "throughput-intercept"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "latency-west", "-a", "loop-west", "-c", "latency-intercept"))

	// Service policies — east hosts bind east services, west clients dial east services (cross-region)
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-hosts-east", "Bind", "--service-roles", "#loop-east", "--identity-roles", "#loop-host-east"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-clients-east", "Dial", "--service-roles", "#loop-east", "--identity-roles", "#loop-client-west"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-hosts-west", "Bind", "--service-roles", "#loop-west", "--identity-roles", "#loop-host-west"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-clients-west", "Dial", "--service-roles", "#loop-west", "--identity-roles", "#loop-client-east"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "loop", "--service-roles", "#loop-east,#loop-west", "--edge-router-roles", "#test"))

	// Sim Services
	workflow.AddAction(zitilib_actions.Edge("create", "service", "metrics", "-a", "sim-services"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "sim-control", "-a", "sim-services"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "sim-service-hosts", "Bind", "--service-roles", "#sim-services", "--identity-roles", "#sim-services-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "sim-service-clients", "Dial", "--service-roles", "#sim-services", "--identity-roles", "#sim-services-client"))

	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "sim-services-hosts", "--edge-router-roles", "#sim-services", "--identity-roles", "#all"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "sim-services", "--service-roles", "#sim-services", "--edge-router-roles", "#sim-services"))

	// Shared policies
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "east", "--edge-router-roles", "#east", "--identity-roles", "#east"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "west", "--edge-router-roles", "#west", "--identity-roles", "#west"))

	// Start remaining controllers (HA join)
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(edge.RaftJoin(".bootstrap.ctrl", ".ctrl"))
	workflow.AddAction(semaphore.Sleep(5 * time.Second))

	// enroll routers after
	workflow.AddAction(edge.InitEdgeRouters(models.RouterTag, 2))

	// Start routers
	workflow.AddAction(component.StartInParallel(models.RouterTag, 10))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Start sim hosts + clients
	workflow.AddAction(component.StartInParallel(".sim-services-host", 50))
	workflow.AddAction(component.StartInParallel(".sim-services-client", 50))

	return workflow
}
