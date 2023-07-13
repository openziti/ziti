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

package actions

import (
	"fmt"
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

	workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/*"))
	workflow.AddAction(component.Stop("#ctrl"))
	workflow.AddAction(component.Exec("#ctrl", zitilab.ControllerActionInitStandalone))
	workflow.AddAction(component.Start("#ctrl"))
	workflow.AddAction(edge.ControllerAvailable("#ctrl", 30*time.Second))

	workflow.AddAction(edge.Login("#ctrl"))

	workflow.AddAction(component.StopInParallel(models.EdgeRouterTag, 25))
	workflow.AddAction(edge.InitEdgeRouters(models.EdgeRouterTag, 2))
	workflow.AddAction(edge.InitIdentities(models.SdkAppTag, 2))

	workflow.AddAction(zitilib_actions.Edge("create", "service", "echo"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "echo-servers", "Bind", "--service-roles", "@echo", "--identity-roles", "#service"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "echo-client", "Dial", "--service-roles", "@echo", "--identity-roles", "#client"))

	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "echo-servers", "--edge-router-roles", "#host", "--identity-roles", "#service"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "echo-clients", "--edge-router-roles", "#client", "--identity-roles", "#client"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-edge-router-policy", "serp-all", "--service-roles", "#all", "--edge-router-roles", "#all"))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "files-host", "host.v1", `
		{
			"address" : "ziti-smoketest-files.s3-us-west-1.amazonaws.com", 
			"port" : 443, 
			"protocol" : "tcp"
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "iperf-host", "host.v1", `
		{
			"address" : "localhost", 
			"port" : 5201, 
			"protocol" : "tcp"
		}`))

	for _, encrypted := range []bool{false, true} {
		for _, hostType := range []string{"ert", "zet", "ziti-tunnel"} {
			suffix := ""
			encryptionFlag := "ON"

			if !encrypted {
				suffix = "-unencrypted"
				encryptionFlag = "OFF"
			}

			filesConfigName := fmt.Sprintf("files-intercept-%s%s", hostType, suffix)
			filesConfigDef := fmt.Sprintf(`
				{
					"addresses": ["files-%s%s.s3-us-west-1.amazonaws.ziti"],
					"portRanges" : [ { "low": 443, "high": 443 } ],
					"protocols": ["tcp"]
				}`, hostType, suffix)

			workflow.AddAction(zitilib_actions.Edge("create", "config", filesConfigName, "intercept.v1", filesConfigDef))

			iperfConfigName := fmt.Sprintf("iperf-intercept-%s%s", hostType, suffix)
			iperfConfigDef := fmt.Sprintf(`
				{
					"addresses": ["iperf-%s%s.ziti"],
					"portRanges" : [ { "low": 5201, "high": 5201 } ],
					"protocols": ["tcp"]
				}`, hostType, suffix)

			workflow.AddAction(zitilib_actions.Edge("create", "config", iperfConfigName, "intercept.v1", iperfConfigDef))

			filesServiceName := fmt.Sprintf("%s-files%s", hostType, suffix)
			filesConfigs := fmt.Sprintf("files-host,%s", filesConfigName)
			workflow.AddAction(zitilib_actions.Edge("create", "service", filesServiceName, "-c", filesConfigs, "-e", encryptionFlag, "-a", hostType))

			iperfServiceName := fmt.Sprintf("%s-iperf%s", hostType, suffix)
			iperfConfigs := fmt.Sprintf("iperf-host,%s", iperfConfigName)
			workflow.AddAction(zitilib_actions.Edge("create", "service", iperfServiceName, "-c", iperfConfigs, "-e", encryptionFlag, "-a", hostType))
		}
	}

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "ert-hosts", "Bind", "--service-roles", "#ert", "--identity-roles", "#ert-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "zet-hosts", "Bind", "--service-roles", "#zet", "--identity-roles", "#zet-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "ziti-tunnel-hosts", "Bind", "--service-roles", "#ziti-tunnel", "--identity-roles", "#ziti-tunnel-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "client-tunnelers", "Dial", "--service-roles", "#all", "--identity-roles", "#client"))

	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "client-routers", "--edge-router-roles", "#client", "--identity-roles", "#client"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "host-routers", "--edge-router-roles", "#host", "--identity-roles", "#host"))

	workflow.AddAction(component.Stop(models.ControllerTag))

	return workflow
}
