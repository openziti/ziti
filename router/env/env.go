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
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
)

type RouterEnv interface {
	GetNetworkControllers() NetworkControllers
	GetRouterId() *identity.TokenId
	GetDialerCfg() map[string]xgress.OptionsData
	GetXlinkDialer() []xlink.Dialer
	GetXrctrls() []Xrctrl
	GetTraceHandler() *channel.TraceHandler
	GetXlinkRegistry() xlink.Registry
	GetCloseNotify() <-chan struct{}
	GetMetricsRegistry() metrics.UsageRegistry
	RenderJsonConfig() (string, error)
	GetHeartbeatOptions() HeartbeatOptions
	GetRateLimiterPool() goroutines.Pool
}
