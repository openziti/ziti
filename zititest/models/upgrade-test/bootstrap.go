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
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
)

type bootstrapAction struct{}

// newBootstrapAction builds the phase-1 bootstrap: a single standalone controller on the
// from-version, enrolled routers and identities, and the loop services that the sim dials
// across all four hosting flavors (SDK, ERT, ZET, ziti-tunnel).
func newBootstrapAction() model.ActionBinder {
	action := &bootstrapAction{}
	return action.bind
}

func (a *bootstrapAction) bind(m *model.Model) model.Action {
	workflow := actions.Workflow()

	// Stop every component first so a re-bootstrap starts from a clean slate. Otherwise sim clients
	// and tunnelers left running from a prior iteration keep stale sessions (and an old-version
	// controller/router process can survive), which fails the next baseline.
	workflow.AddAction(component.StopInParallel("*", 25))

	// Only ctrl1 runs in phase 1. ctrl2/ctrl3 are provisioned but idle until the add-nodes phase.
	workflow.AddAction(component.StopInParallel(".ctrl", 15))
	workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/*"))
	workflow.AddAction(host.GroupExec("component.ctrl", 5, "rm -rf ./fablab/ctrldata ./fablab/ctrl.db"))

	workflow.AddAction(component.Exec("#ctrl1", zitilab.ControllerActionInitStandalone))
	workflow.AddAction(component.Start("#ctrl1"))
	workflow.AddAction(edge.ControllerAvailable("#ctrl1", 30*time.Second))
	workflow.AddAction(edge.Login("#ctrl1"))

	workflow.AddAction(component.StopInParallel(models.EdgeRouterTag, 25))
	workflow.AddAction(edge.InitEdgeRouters(models.EdgeRouterTag, 2))
	workflow.AddAction(edge.InitIdentities(models.SdkAppTag, 2))

	// Shared host.v1 config: every tunneler-hosted loop service forwards to a local loop4
	// listener on tcp:127.0.0.1:3456.
	workflow.AddAction(zitilib_actions.Edge("create", "config", "loop-backend", "host.v1", `
		{
			"address" : "localhost",
			"port" : 3456,
			"protocol" : "tcp"
		}`))

	// intercept.v1 for loop-zet: ZET is tproxy-only (no proxy-port mode), so it needs an intercept
	// config to know the address/port to intercept. The Go ziti-tunnel and ERT clients use proxy mode
	// with the port on the command line, so they need no intercept config. Use a hostname (not a bare
	// IP): ZET's tproxy assigns an IP from its own DNS range for the hostname and intercepts that; a
	// static IP isn't intercepted. The co-located loop4 dialer targets this hostname, which resolves
	// through ZET's resolver into the intercept.
	workflow.AddAction(zitilib_actions.Edge("create", "config", "loop-zet-intercept", "intercept.v1", `
		{
			"addresses": ["loop-zet.ziti"],
			"portRanges": [{ "low": 15391, "high": 15391 }],
			"protocols": ["tcp"]
		}`))

	// One loop service per hosting flavor.
	workflow.AddAction(zitilib_actions.Edge("create", "service", "loop-sdk", "-a", "loop-svc,loop-sdk-host-svc"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "loop-ert", "-c", "loop-backend", "-a", "loop-svc,loop-ert-svc"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "loop-zet", "-c", "loop-backend,loop-zet-intercept", "-a", "loop-svc,loop-zet-svc"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "loop-ziti-tunnel", "-c", "loop-backend", "-a", "loop-svc,loop-zt-svc"))

	// Bind policies: each flavor's service is hosted by the matching host identity.
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-sdk-hosts", "Bind", "--service-roles", "#loop-sdk-host-svc", "--identity-roles", "#loop-sdk-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-ert-hosts", "Bind", "--service-roles", "#loop-ert-svc", "--identity-roles", "#ert-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-zet-hosts", "Bind", "--service-roles", "#loop-zet-svc", "--identity-roles", "#zet-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-ziti-tunnel-hosts", "Bind", "--service-roles", "#loop-zt-svc", "--identity-roles", "#ziti-tunnel-host"))

	// Dial policy: the sim client dials every loop service.
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-clients", "Dial", "--service-roles", "#loop-svc", "--identity-roles", "#loop-client"))

	// Dial policy: the ziti-tunnel proxy clients dial loop-ziti-tunnel, so a co-located loop4 dialer can
	// push traffic through the tunneler's local proxy listener (exercises the ziti-tunnel client path).
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-ziti-tunnel-clients", "Dial", "--service-roles", "#loop-zt-svc", "--identity-roles", "#ziti-tunnel-client"))

	// Dial policy: the ERT proxy client (router-east-1) dials loop-ert, so its proxy listener + a
	// co-located loop4 dialer exercise the ERT client path.
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-ert-clients", "Dial", "--service-roles", "#loop-ert-svc", "--identity-roles", "#ert-proxy-client"))

	// Dial policy: the ZET clients dial loop-zet so their tproxy intercept + a co-located loop4 dialer
	// exercise the ZET client path.
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "loop-zet-clients", "Dial", "--service-roles", "#loop-zet-svc", "--identity-roles", "#zet-client"))

	// Sim control-plane services (metrics reporting + scenario control), hosted by the
	// sim-controller identity created during activation.
	workflow.AddAction(zitilib_actions.Edge("create", "service", "metrics", "-a", "sim-services"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "sim-control", "-a", "sim-services"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "sim-service-hosts", "Bind", "--service-roles", "#sim-services", "--identity-roles", "#sim-services-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "sim-service-clients", "Dial", "--service-roles", "#sim-services", "--identity-roles", "#sim-services-client"))

	// Broad routing: every identity may use every edge router, and every service may be
	// routed over every edge router. Simple and sufficient for the upgrade test.
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "all-endpoints", "--edge-router-roles", "#all", "--identity-roles", "#all"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "all-services", "--service-roles", "#all", "--edge-router-roles", "#all"))

	return workflow
}
