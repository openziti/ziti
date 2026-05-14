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

package metrics

// HostStatsConfig configures host stats registration.
//
// Host stats are emitted only on Linux; on other operating systems
// RegisterHostStats logs a warning and returns. The implementation lives
// in build-tagged files (host_stats_linux.go / host_stats_other.go) so
// that gopsutil and its per-OS code paths are not linked into non-Linux
// builds.
type HostStatsConfig struct {
	// Enabled toggles registration. When false RegisterHostStats is a no-op
	// on every platform.
	Enabled bool
}
