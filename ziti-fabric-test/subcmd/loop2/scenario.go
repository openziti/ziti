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

package loop2

import (
	"gopkg.in/yaml.v2"
	"os"
)

type Scenario struct {
	Workloads       []*Workload `yaml:"workloads"`
	ConnectionDelay int32       `yaml:"connectionDelay"`
}

type Workload struct {
	Name        string `yaml:"name"`
	Concurrency int32  `yaml:"concurrency"`
	Dialer      Test   `yaml:"dialer"`
	Listener    Test   `yaml:"listener"`
}

type Test struct {
	TxRequests      int32 `yaml:"txRequests"`
	TxPacing        int32 `yaml:"txPacing"`
	TxMaxJitter     int32 `yaml:"txMaxJitter"`
	RxTimeout       int32 `yaml:"rxTimeout"`
	PayloadMinBytes int32 `yaml:"payloadMinBytes"`
	PayloadMaxBytes int32 `yaml:"payloadMaxBytes"`
}

func LoadScenario(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	scenario := &Scenario{ConnectionDelay: 250}
	if err := yaml.Unmarshal(data, scenario); err != nil {
		return nil, err
	}

	return scenario, nil
}

func (scenario *Scenario) String() string {
	data, err := yaml.Marshal(scenario)
	if err != nil {
		panic(err)
	}
	return string(data)
}
