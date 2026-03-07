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
	"fmt"
	"math/rand"
	"time"

	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab/chaos"
)

// start with a random scenario then cycle through them
var scenarioCounter = rand.Intn(7)

func sowChaos(run model.Run) error {
	var controllers []*model.Component
	var err error

	scenarioCounter = (scenarioCounter + 1) % 7
	scenario := scenarioCounter + 1

	if scenario&0b001 > 0 {
		controllers, err = chaos.SelectRandom(run, ".ctrl", chaos.RandomOfTotal())
		if err != nil {
			return err
		}
		time.Sleep(5 * time.Second)
	}

	var routers []*model.Component
	if scenario&0b010 > 0 {
		routers, err = chaos.SelectRandom(run, ".router", chaos.PercentageRange(10, 75))
		if err != nil {
			return err
		}
	}

	var hosts []*model.Component
	if scenario&0b100 > 0 {
		hosts, err = chaos.SelectRandom(run, ".host", chaos.PercentageRange(10, 75))
		if err != nil {
			return err
		}
	}

	toRestart := append(([]*model.Component)(nil), controllers...)
	toRestart = append(toRestart, routers...)
	toRestart = append(toRestart, hosts...)
	fmt.Printf("restarting %d controllers,  %d routers and %d hosts\n", len(controllers), len(routers), len(hosts))
	return chaos.RestartSelected(run, 100, toRestart...)
}
