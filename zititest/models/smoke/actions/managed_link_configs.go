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

	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
)

// createManagedLinkConfigsAction creates a router.link.v1 config for each
// component matching componentSpec (typically ".ctrl-managed-link") and
// assigns it to the matching edge router. Each config carries listener +
// dialer settings equivalent to what the local YAML would have produced,
// so the converted routers come up with the same link surface as their
// non-managed peers but via controller-managed delivery.
//
// Must run after edge router entities have been created/enrolled
// (InitEdgeRouters) and before routers start, so that by the time
// each router subscribes to the RDM, its Configs assignment is in place.
type createManagedLinkConfigsAction struct {
	componentSpec string
}

// CreateManagedLinkConfigs returns the action.
func CreateManagedLinkConfigs(componentSpec string) model.Action {
	return &createManagedLinkConfigsAction{componentSpec: componentSpec}
}

func (a *createManagedLinkConfigsAction) Execute(run model.Run) error {
	for _, c := range run.GetModel().SelectComponents(a.componentSpec) {
		if c.Host == nil {
			return fmt.Errorf("component %q has no host", c.Id)
		}
		configName := "link-" + c.Id
		data := fmt.Sprintf(`{
			"listeners": [{
				"binding": "transport",
				"bind": "tls:0.0.0.0:6000",
				"advertise": "tls:%s:6000"
			}],
			"dialers": [{
				"binding": "transport",
				"options": {"connectTimeout": "30s"}
			}]
		}`, c.Host.PublicIp)

		if err := zitilib_actions.EdgeExec(run.GetModel(), "create", "config", configName, "router.link.v1", data); err != nil {
			return fmt.Errorf("create link config for %q: %w", c.Id, err)
		}
		if err := zitilib_actions.EdgeExec(run.GetModel(), "update", "edge-router", c.Id, "--configs", configName); err != nil {
			return fmt.Errorf("assign link config to %q: %w", c.Id, err)
		}
	}
	return nil
}
