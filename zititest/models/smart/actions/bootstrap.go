/*
	Copyright 2020 NetFoundry Inc.

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
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"github.com/sirupsen/logrus"
	"path/filepath"
	"time"
)

func NewBootstrapAction() model.ActionBinder {
	action := &bootstrapAction{}
	return action.bind
}

func (self *bootstrapAction) bind(m *model.Model) model.Action {
	workflow := actions.Workflow()

	workflow.AddAction(component.Stop(models.ControllerTag))
	workflow.AddAction(host.Exec(m.MustSelectHost(models.HasControllerComponent), "rm -f ~/ctrl.db"))
	workflow.AddAction(component.Start(models.ControllerTag))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	for _, router := range m.SelectComponents(models.RouterTag) {
		cert := fmt.Sprintf("/intermediate/certs/%s-client.cert", router.Id)
		workflow.AddAction(zitilib_actions.Fabric("create", "router", filepath.Join(model.PkiBuild(), cert)))
	}

	serviceActions, err := self.createServiceActions(m)
	if err != nil {
		logrus.Fatalf("error creating service actions (%v)", err)
	}
	for _, serviceAction := range serviceActions {
		workflow.AddAction(serviceAction)
	}

	sshUsername := m.MustStringVariable("credentials.ssh.username")
	for _, h := range m.SelectHosts("*") {
		workflow.AddAction(host.Exec(h, fmt.Sprintf("mkdir -p /home/%s/.ziti", sshUsername)))
		workflow.AddAction(host.Exec(h, fmt.Sprintf("rm -f /home/%s/.ziti/identities.yml", sshUsername)))
		workflow.AddAction(host.Exec(h, fmt.Sprintf("ln -s /home/%s/fablab/cfg/remote_identities.yml /home/%s/.ziti/identities.yml", sshUsername, sshUsername)))
	}

	workflow.AddAction(component.Stop(models.ControllerTag))

	return workflow
}

func (_ *bootstrapAction) createServiceActions(m *model.Model) ([]model.Action, error) {
	var serviceActions []model.Action
	hosts, err := m.MustSelectHosts(models.LoopListenerTag, 1)
	if err != nil {
		return nil, err
	}

	router, err := m.SelectComponent(".router.terminator")
	if err != nil {
		return nil, err
	}

	for _, h := range hosts {
		serviceActions = append(serviceActions, zitilib_actions.Fabric("create", "service", h.GetId()))
		serviceActions = append(serviceActions, zitilib_actions.Fabric("create", "terminator", h.GetId(), router.Id, "tcp:"+h.PrivateIp+":8171"))
	}

	return serviceActions, nil
}

type bootstrapAction struct{}
