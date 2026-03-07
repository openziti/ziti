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

package inspect

// CtrlDialerKey is the inspection key used to retrieve ctrl dialer state via the inspect API.
const CtrlDialerKey = "ctrl-dialer"

// CtrlDialerInspectResult is the top-level result returned when inspecting the ctrl dialer.
type CtrlDialerInspectResult struct {
	Enabled bool                    `json:"enabled"`
	Config  CtrlDialerConfigDetail  `json:"config"`
	Routers []*CtrlDialerRouterDial `json:"routers"`
}

// CtrlDialerConfigDetail contains the active configuration of the ctrl dialer.
type CtrlDialerConfigDetail struct {
	Groups             []string `json:"groups"`
	DialDelay          string   `json:"dialDelay"`
	MinRetryInterval   string   `json:"minRetryInterval"`
	MaxRetryInterval   string   `json:"maxRetryInterval"`
	RetryBackoffFactor float64  `json:"retryBackoffFactor"`
	FastFailureWindow  string   `json:"fastFailureWindow"`
	QueueSize          uint32   `json:"queueSize"`
	MaxWorkers         uint32   `json:"maxWorkers"`
}

// CtrlDialerRouterDial describes the current dial state for a single router.
type CtrlDialerRouterDial struct {
	RouterId       string   `json:"routerId"`
	Addresses      []string `json:"addresses"`
	CurrentAddress string   `json:"currentAddress"`
	Status         string   `json:"status"`
	DialAttempts   uint32   `json:"dialAttempts"`
	RetryDelay     string   `json:"retryDelay"`
	NextDial       string   `json:"nextDial"`
}
