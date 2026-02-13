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

package env

import (
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/inspect"
)

// MockNetworkControllers implements env.NetworkControllers for testing
type MockNetworkControllers struct {
	Channel channel.Channel
}

func (m *MockNetworkControllers) AnyChannel() channel.Channel {
	return m.Channel
}

func (m *MockNetworkControllers) AnyCtrlChannel() ctrlchan.CtrlChannel {
	panic("implement me")
}

func (m *MockNetworkControllers) GetCtrlChannel(ctrlId string) ctrlchan.CtrlChannel {
	return nil
}

func (m *MockNetworkControllers) UpdateControllerEndpoints(endpoints []string) bool {
	return false
}

func (m *MockNetworkControllers) UpdateLeader(leaderId string) {
}

func (m *MockNetworkControllers) GetAll() map[string]NetworkController {
	return nil
}

func (m *MockNetworkControllers) GetNetworkController(ctrlId string) NetworkController {
	return nil
}

func (m *MockNetworkControllers) GetModelUpdateCtrlChannel() channel.Channel {
	return m.Channel
}

func (m *MockNetworkControllers) GetIfResponsive(ctrlId string) (channel.Channel, bool) {
	return m.Channel, true
}

func (m *MockNetworkControllers) AllResponsiveCtrlChannels() []channel.Channel {
	return []channel.Channel{m.Channel}
}

func (m *MockNetworkControllers) AnyValidCtrlChannel() channel.Channel {
	return m.Channel
}

func (m *MockNetworkControllers) GetChannel(ctrlId string) channel.Channel {
	return m.Channel
}

func (m *MockNetworkControllers) DefaultRequestTimeout() time.Duration {
	return time.Second
}

func (m *MockNetworkControllers) ForEach(f func(ctrlId string, ch channel.Channel)) {
	f("test", m.Channel)
}

func (m *MockNetworkControllers) Close() error {
	return nil
}

func (m *MockNetworkControllers) Inspect() *inspect.ControllerInspectDetails {
	return nil
}

func (m *MockNetworkControllers) AddChangeListener(listener CtrlEventListener) {
}

func (m *MockNetworkControllers) NotifyOfDisconnect(ctrlId string) {
}

func (m *MockNetworkControllers) NotifyOfReconnect(ctrlId string) {
}

func (m *MockNetworkControllers) GetExpectedCtrlCount() uint32 {
	return 1
}

func (m *MockNetworkControllers) IsLeaderConnected() bool {
	return true
}

func (m *MockNetworkControllers) ControllersHaveMinVersion(version string) bool {
	return true
}

func (m *MockNetworkControllers) GetLeader() NetworkController {
	return nil
}

func (m *MockNetworkControllers) AcceptCtrlChannel(address string, ctrlCh ctrlchan.CtrlChannel, binding channel.Binding, underlay channel.Underlay) error {
	return nil
}
