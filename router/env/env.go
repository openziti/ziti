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
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/config"
	"github.com/openziti/ziti/router/xgress_router"
	"github.com/openziti/ziti/router/xlink"
	"time"
)

type RouterEnv interface {
	GetNetworkControllers() NetworkControllers
	GetRouterId() *identity.TokenId
	GetDialerCfg() map[string]xgress.OptionsData
	GetXlinkDialers() []xlink.Dialer
	GetXgressListeners() []xgress_router.Listener
	GetXrctrls() []Xrctrl
	GetTraceHandler() *channel.TraceHandler
	GetXlinkRegistry() xlink.Registry
	GetCloseNotify() <-chan struct{}
	GetMetricsRegistry() metrics.UsageRegistry
	RenderJsonConfig() (string, error)
	GetHeartbeatOptions() HeartbeatOptions
	GetRateLimiterPool() goroutines.Pool
	GetCtrlRateLimiter() rate.AdaptiveRateLimitTracker
	GetVersionInfo() versions.VersionProvider
	GetRouterDataModel() *common.RouterDataModel
	GetConnectEventsConfig() *ConnectEventsConfig
	GetRouterDataModelEnabledConfig() *config.Value[bool]
	IsRouterDataModelRequired() bool
	MarkRouterDataModelRequired()
	GetIndexWatchers() IndexWatchers
	IsRouterDataModelEnabled() bool
	DefaultRequestTimeout() time.Duration
	GetXgressBindHandler() xgress.BindHandler
	GetConfig() *Config
	GetForwarder() Forwarder
	GetXgressMetrics() XgressMetrics
}

type ConnectEventsConfig struct {
	Enabled          bool
	BatchInterval    time.Duration
	MaxQueuedEvents  int64
	FullSyncInterval time.Duration
}

type Forwarder interface {
	ForwardPayload(srcAddr xgress.Address, payload *xgress.Payload, timeout time.Duration) error
	ForwardAcknowledgement(srcAddr xgress.Address, acknowledgement *xgress.Acknowledgement) error
	ForwardControl(srcAddr xgress.Address, control *xgress.Control) error
	ReportForwardingFault(circuitId string, ctrlId string)
	RegisterDestination(circuitId string, address xgress.Address, destination Destination)
	EndCircuit(circuitId string)
}

type Destination interface {
	SendPayload(payload *xgress.Payload, timeout time.Duration, payloadType xgress.PayloadType) error
	SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error
	SendControl(control *xgress.Control) error
	InspectCircuit(detail *xgress.CircuitInspectDetail)
}

type XgressMetrics interface {
	Rx(source metrics.UsageSource, originator xgress.Originator, payload *xgress.Payload)
	Tx(source metrics.UsageSource, originator xgress.Originator, payload *xgress.Payload)
}
