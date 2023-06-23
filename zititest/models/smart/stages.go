/*
	Copyright 2019 NetFoundry Inc.

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
	"fmt"
	aws_ssh_keys0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	"github.com/openziti/fablab/kernel/lib/runlevel/1_configuration/config"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	fablib_5_operation "github.com/openziti/fablab/kernel/lib/runlevel/5_operation"
	aws_ssh_keys6 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	terraform6 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/models"
	zitilab_5_operation "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
	"time"
)

func newStageFactory() model.Factory {
	return &stageFactory{}
}

// stageFactory is a model.Factory that is responsible for building and connecting the stages that
// represent the various phases of the model.
func (self *stageFactory) Build(m *model.Model) error {
	// set up ssh keys (if configured), create the cloud infrastructure and restart all instances after they've been updated
	m.Infrastructure = model.Stages{
		aws_ssh_keys0.Express(),
		terraform0.Express(),
		semaphore0.Restart(90 * time.Second),
	}

	// pull together the artifacts that will be distributed
	// This includes
	// 1. keys and certs
	// 2. component configurations, which includes controller and router configurations
	// 3. loop tester configurations
	// 4. The actual binaries, including the controller, router, ziti-fabric CLI and the ziti-fabric-test tool
	m.Configuration = model.Stages{
		config.Static([]config.StaticConfig{
			{Src: "10-ambient.loop2.yml", Name: "10-ambient.loop2.yml"},
			{Src: "4k-chatter.loop2.yml", Name: "4k-chatter.loop2.yml"},
			{Src: "remote_identities.yml", Name: "remote_identities.yml"},
		}),
	}

	// Create log directories on hosts that need them
	// Push the artifacts gather in the configuration stage up to the hosts
	m.Distribution = model.Stages{
		distribution.Locations(models.HasControllerComponent, "logs"),
		distribution.Locations(models.HasRouterComponent, "logs"),
		distribution.Locations(models.LoopListenerTag, "logs"),
		distribution.Locations(models.LoopDialerTag, "logs"),
		rsync.Rsync(1),
	}

	// Run the bootstrap and start actions
	// This will
	// 1. reset the controller
	// 2. enroll the routers
	// 3. create the service and terminator
	// 4. Create config files that the ziti-fabric-test loop tool can use to connect and send data
	// 5. Ensure the controller and router are started
	m.AddActivationActions("bootstrap", "start")

	// In the operating phase, this model launches mesh structure polling, fabric metrics listening, and then creates the correct
	// loop2 dialers and listeners that run against the model. When the dialers complete their operation, the joiner will
	// join with them, invoking the closer and ending the mesh and metrics pollers. Finally, the instance state is
	// persisted as a dump.
	if err := self.addOperationStages(m); err != nil {
		return err
	}

	// finally, dispose of the instances and remove any added keys
	m.Disposal = model.Stages{
		terraform6.Dispose(),
		aws_ssh_keys6.Dispose(),
	}

	return nil
}

func (self *stageFactory) addOperationStages(m *model.Model) error {
	phase := fablib_5_operation.NewPhase()

	m.AddOperatingStage(zitilab_5_operation.Mesh(phase.GetCloser()))
	//m.AddOperatingStage(zitilab_5_operation.Metrics(phase.GetCloser()))

	if err := self.addListenerStages(m); err != nil {
		return fmt.Errorf("error creating listeners (%w)", err)
	}

	m.AddOperatingStage(fablib_5_operation.Timer(5*time.Second, nil))

	if err := self.dialers(m, phase); err != nil {
		return fmt.Errorf("error creating dialers (%w)", err)
	}

	m.AddOperatingStage(phase)
	m.AddOperatingStage(fablib_5_operation.Persist())

	return nil
}

func (_ *stageFactory) addListenerStages(m *model.Model) error {
	hosts := m.SelectHosts(models.LoopListenerTag)
	if len(hosts) < 1 {
		return fmt.Errorf("no '%v' hosts in model", models.LoopListenerTag)
	}

	for _, host := range hosts {
		m.AddOperatingStage(zitilab_5_operation.LoopListener(host, nil, "tcp:0.0.0.0:8171"))
	}

	return nil
}

func (_ *stageFactory) dialers(m *model.Model, phase fablib_5_operation.Phase) error {
	initiator, err := m.SelectHost("component.initiator.router")
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("tls:%s:7002", initiator.PublicIp)

	hosts, err := m.MustSelectHosts(models.LoopDialerTag, 1)
	if err != nil {
		return err
	}

	for _, host := range hosts {
		m.AddOperatingStage(zitilab_5_operation.LoopDialer(host, "10-ambient.loop2.yml", endpoint, phase.AddJoiner()))
	}

	return nil
}

type stageFactory struct{}
