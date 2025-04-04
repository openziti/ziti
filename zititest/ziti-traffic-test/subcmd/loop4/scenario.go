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

package loop4

import (
	"fmt"
	loopPb "github.com/openziti/ziti/zititest/ziti-traffic-test/subcmd/loop4/pb"
	"gopkg.in/yaml.v2"
	"os"
	"time"
)

type Connector struct {
	SdkOptions       *SdkConnectOptions       `yaml:"sdk"`
	TransportOptions *TransportConnectOptions `yaml:"transport"`
}

type SdkConnectOptions struct {
	IdentityFile        string `yaml:"identity_file"`
	DisableMultiChannel bool   `yaml:"disable_multi_channel"`
}

type TransportConnectOptions struct {
	Address      string `yaml:"address"`
	IdentityFile string `yaml:"identity_file"`
}

type Scenario struct {
	ConnectorConfigs map[string]Connector `yaml:"connectors"`
	Workloads        []*Workload          `yaml:"workloads"`
	ConnectionDelay  int32                `yaml:"connectionDelay"`
	Metrics          *Metrics             `yaml:"metrics"`
}

type Workload struct {
	Name           string        `yaml:"name"`
	Connector      string        `yaml:"connector"`
	ConnectTimeout time.Duration `yaml:"connect_timeout"`
	ServiceName    string        `yaml:"service_name"`
	Iterations     int64         `yaml:"iterations"`
	Concurrency    int32         `yaml:"concurrency"`
	Dialer         Test          `yaml:"dialer"`
	Listener       Test          `yaml:"listener"`
}

func (self *Workload) GetRunnerName(index int) string {
	return fmt.Sprintf("%s:%d", self.Name, index)
}

type Test struct {
	TxRequests   int32         `yaml:"txRequests"`
	TxPacing     time.Duration `yaml:"txPacing"`
	TxMaxJitter  time.Duration `yaml:"txMaxJitter"`
	TxPauseEvery time.Duration `yaml:"txPauseEvery"`
	TxPauseFor   time.Duration `yaml:"txPauseFor"`
	TxAfterRx    bool          `yaml:"txAfterRx"` // only send after receive, to act like a server responding to requests

	RxTimeout    int32         `yaml:"rxTimeout"`
	RxPacing     time.Duration `yaml:"rxPacing"`
	RxMaxJitter  time.Duration `yaml:"rxMaxJitter"`
	RxPauseEvery time.Duration `yaml:"rxPauseEvery"`
	RxPauseFor   time.Duration `yaml:"rxPauseFor"`

	PayloadMinBytes  int32  `yaml:"payloadMinBytes"`
	PayloadMaxBytes  int32  `yaml:"payloadMaxBytes"`
	LatencyFrequency int32  `yaml:"latencyFrequency"`
	BlockType        string `yaml:"blockType"`
}

func (workload *Workload) GetTests() (*loopPb.Test, *loopPb.Test) {
	local := &loopPb.Test{
		Name:             workload.Name,
		TxRequests:       workload.Dialer.TxRequests,
		TxPacing:         workload.Dialer.TxPacing.String(),
		TxMaxJitter:      workload.Dialer.TxMaxJitter.String(),
		TxPauseEvery:     workload.Dialer.TxPauseEvery.String(),
		TxPauseFor:       workload.Dialer.TxPauseFor.String(),
		TxAfterRx:        workload.Dialer.TxAfterRx,
		RxRequests:       workload.Listener.TxRequests,
		RxPacing:         workload.Dialer.RxPacing.String(),
		RxMaxJitter:      workload.Dialer.RxMaxJitter.String(),
		RxPauseEvery:     workload.Dialer.RxPauseEvery.String(),
		RxPauseFor:       workload.Dialer.RxPauseFor.String(),
		RxTimeout:        workload.Dialer.RxTimeout,
		RxSeqBlockSize:   workload.Listener.PayloadMinBytes,
		PayloadMinBytes:  workload.Dialer.PayloadMinBytes,
		PayloadMaxBytes:  workload.Dialer.PayloadMaxBytes,
		LatencyFrequency: workload.Dialer.LatencyFrequency,
		TxBlockType:      workload.Dialer.BlockType,
		RxBlockType:      workload.Listener.BlockType,
	}

	remote := &loopPb.Test{
		Name:             workload.Name,
		TxRequests:       workload.Listener.TxRequests,
		TxPacing:         workload.Listener.TxPacing.String(),
		TxMaxJitter:      workload.Listener.TxMaxJitter.String(),
		TxPauseEvery:     workload.Listener.TxPauseEvery.String(),
		TxPauseFor:       workload.Listener.TxPauseFor.String(),
		TxAfterRx:        workload.Listener.TxAfterRx,
		RxRequests:       workload.Dialer.TxRequests,
		RxPacing:         workload.Listener.RxPacing.String(),
		RxMaxJitter:      workload.Listener.RxMaxJitter.String(),
		RxTimeout:        workload.Listener.RxTimeout,
		RxPauseEvery:     workload.Listener.RxPauseEvery.String(),
		RxPauseFor:       workload.Listener.RxPauseFor.String(),
		RxSeqBlockSize:   workload.Dialer.PayloadMinBytes,
		PayloadMinBytes:  workload.Listener.PayloadMinBytes,
		PayloadMaxBytes:  workload.Listener.PayloadMaxBytes,
		LatencyFrequency: workload.Listener.LatencyFrequency,
		TxBlockType:      workload.Listener.BlockType,
		RxBlockType:      workload.Dialer.BlockType,
	}

	return local, remote
}

type Metrics struct {
	Connector      string        `yaml:"connector"`
	Service        string        `yaml:"service"`
	ReportInterval time.Duration `yaml:"interval"`
	ClientId       string        `yaml:"clientId"`
}

func LoadScenario(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	scenario := &Scenario{}
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
