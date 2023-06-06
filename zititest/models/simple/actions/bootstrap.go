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

	workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/*"))
	workflow.AddAction(component.Stop("#ctrl"))
	workflow.AddAction(edge.InitController("#ctrl"))
	workflow.AddAction(component.Start("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

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

	workflow.AddAction(zitilib_actions.Edge("create", "config", "files-intercept-ert-unencrypted", "intercept.v1", `
		{
			"addresses": ["ziti-files-ert-unencrypted.s3-us-west-1.amazonaws.ziti"],
			"portRanges" : [ { "low": 443, "high": 443 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "files-intercept-ert", "intercept.v1", `
		{
			"addresses": ["ziti-files-ert.s3-us-west-1.amazonaws.ziti"],
			"portRanges" : [ { "low": 443, "high": 443 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "files-intercept-zet-unencrypted", "intercept.v1", `
		{
			"addresses": ["ziti-files-zet-unencrypted.s3-us-west-1.amazonaws.ziti"],
			"portRanges" : [ { "low": 443, "high": 443 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "files-intercept-zet", "intercept.v1", `
		{
			"addresses": ["ziti-files-zet.s3-us-west-1.amazonaws.ziti"],
			"portRanges" : [ { "low": 443, "high": 443 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "files-intercept-ziti-tunnel-unencrypted", "intercept.v1", `
		{
			"addresses": ["ziti-files-ziti-tunnel-unencrypted.s3-us-west-1.amazonaws.ziti"],
			"portRanges" : [ { "low": 443, "high": 443 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "config", "files-intercept-ziti-tunnel", "intercept.v1", `
		{
			"addresses": ["ziti-files-ziti-tunnel.s3-us-west-1.amazonaws.ziti"],
			"portRanges" : [ { "low": 443, "high": 443 } ],
			"protocols": ["tcp"]
		}`))

	workflow.AddAction(zitilib_actions.Edge("create", "service", "ert-files-unencrypted", "-c", "files-host,files-intercept-ert-unencrypted", "-e", "OFF", "-a", "ert"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "ert-files", "-c", "files-host,files-intercept-ert", "-a", "ert"))

	workflow.AddAction(zitilib_actions.Edge("create", "service", "zet-files-unencrypted", "-c", "files-host,files-intercept-zet-unencrypted", "-e", "OFF", "-a", "zet"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "zet-files", "-c", "files-host,files-intercept-zet", "-a", "zet"))

	workflow.AddAction(zitilib_actions.Edge("create", "service", "ziti-tunnel-files-unencrypted", "-c", "files-host,files-intercept-ziti-tunnel-unencrypted", "-e", "OFF", "-a", "ziti-tunnel"))
	workflow.AddAction(zitilib_actions.Edge("create", "service", "ziti-tunnel-files", "-c", "files-host,files-intercept-ziti-tunnel", "-a", "ziti-tunnel"))

	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "ert-hosts", "Bind", "--service-roles", "#ert", "--identity-roles", "#ert-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "zet-hosts", "Bind", "--service-roles", "#zet", "--identity-roles", "#zet-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "ziti-tunnel-hosts", "Bind", "--service-roles", "#ziti-tunnel", "--identity-roles", "#ziti-tunnel-host"))
	workflow.AddAction(zitilib_actions.Edge("create", "service-policy", "client-tunnelers", "Dial", "--service-roles", "#all", "--identity-roles", "#client"))

	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "client-routers", "--edge-router-roles", "#client", "--identity-roles", "#client"))
	workflow.AddAction(zitilib_actions.Edge("create", "edge-router-policy", "host-routers", "--edge-router-roles", "#host", "--identity-roles", "#host"))

	workflow.AddAction(component.Stop(models.ControllerTag))

	workflow.AddAction(model.ActionFunc(func(m *model.Model) error {
		cmds := []string{
			"sudo sed -i 's/#DNS=/DNS=127.0.0.1/g' /etc/systemd/resolved.conf",
			"sudo systemctl restart systemd-resolved",
		}

		return m.ForEachComponent(".ziti-tunnel", 2, func(c *model.Component) error {
			return host.Exec(c.GetHost(), cmds...).Execute(c.GetModel())
		})
	}))

	return workflow
}
