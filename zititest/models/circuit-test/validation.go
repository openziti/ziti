/*
	Copyright NetFoundry Inc.

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

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/fablab/kernel/model"
	loop4Pb "github.com/openziti/ziti/zititest/ziti-traffic-test/loop4/pb"
	"github.com/openziti/ziti/zititest/zitilab"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
	zitiLibOps "github.com/openziti/ziti/zititest/zitilab/runlevel/5_operation"
)

type simCallback struct {
	ctrlClients *chaos.CtrlClients
}

func (self *simCallback) DiagnosticRequested(msg *channel.Message, ch channel.Channel) {
	circuitId, _ := msg.GetStringHeader(int32(loop4Pb.HeaderType_RequestIdHeader))
	inspectKeys := []string{"stackdump", "circuitAndStacks:" + circuitId}

	err := self.ctrlClients.InspectAndWriteToFile("ctrl1", ".*", "/home/plorenz/work/support/flow/"+circuitId, "yaml", inspectKeys...)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("failed to run inspect, diagnostic requested by circuit  %s", circuitId)
	} else {
		pfxlog.Logger().Infof("inspect run, diagnostic requested by circuit  %s", circuitId)
	}
}

func RunSimScenarios(run model.Run, services *zitiLibOps.SimServices) error {
	ctrlClients, err := chaos.NewCtrlClients(run, "#ctrl1")
	if err != nil {
		return err
	}

	if err := run.GetModel().Exec(run, "startSimMetrics"); err != nil {
		return err
	}

	cb := &simCallback{
		ctrlClients: ctrlClients,
	}
	simControl, err := services.GetSimController(run, "sim-control", cb)
	if err != nil {
		return err
	}

	sims := run.GetModel().FilterComponents(".loop-client", func(c *model.Component) bool {
		t, ok := c.Type.(*zitilab.Loop4SimType)
		return ok && t.Mode == zitilab.Loop4RemoteControlled
	})

	err = simControl.WaitForAllConnected(time.Second*30, sims)
	if err != nil {
		return err
	}

	results, err := simControl.StartSimScenarios()
	if err != nil {
		return err
	}

	return results.GetResults(5 * time.Minute)
}
