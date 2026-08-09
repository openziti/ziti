//go:build !linux

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

package servermetrics

import (
	"runtime"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/metrics"
)

// RegisterHostStats is the non-Linux stub. The Linux implementation reads
// /proc to populate the metrics documented in host_stats_linux.go; the same
// data is not uniformly available on other operating systems, and silently
// returning zeros for unsupported fields was deemed more confusing than
// useful. When hostMetrics.enabled is true on a non-Linux build, this logs a
// one-time warning and returns without registering anything.
func RegisterHostStats(_ metrics.Registry, cfg HostStatsConfig) {
	if !cfg.Enabled {
		return
	}
	pfxlog.Logger().
		WithField("goos", runtime.GOOS).
		Warn("hostMetrics.enabled is set but host stats are only emitted on Linux; ignoring")
}
