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

package config

import (
	"fmt"

	"github.com/openziti/channel/v4"
)

// ValidateHeartbeatOptions reports whether options describe a cadence that can keep a channel up.
// Callers pass loaded configuration, so the returned error names the configuration keys and is
// suitable for returning straight out of config loading.
//
// Apply this wherever the heartbeat callback closes the channel after CloseUnresponsiveTimeout
// without a response. A callback that only records latency has no deadline to sample and does not
// need it. A zero CloseUnresponsiveTimeout disables the teardown, and only the check interval is
// constrained.
func ValidateHeartbeatOptions(options *channel.HeartbeatOptions) error {
	if options == nil {
		return nil
	}

	// ConfigureHeartbeat hands this straight to time.NewTicker on a bare goroutine, which panics on
	// a non-positive interval and takes the process with it.
	if options.CheckInterval <= 0 {
		return fmt.Errorf("heartbeat checkInterval (%v) must be greater than zero", options.CheckInterval)
	}

	if options.CloseUnresponsiveTimeout <= 0 {
		return nil
	}

	// The check interval is the sampling rate for the timeout, and the response time it reads can be
	// a full interval stale, so at or above the timeout it condemns channels that are answering.
	if options.CheckInterval >= options.CloseUnresponsiveTimeout {
		return fmt.Errorf("heartbeat checkInterval (%v) must be less than closeUnresponsiveTimeout (%v), "+
			"otherwise the check reads a stale response time and closes healthy channels",
			options.CheckInterval, options.CloseUnresponsiveTimeout)
	}

	if options.SendInterval >= options.CloseUnresponsiveTimeout {
		return fmt.Errorf("heartbeat sendInterval (%v) must be less than closeUnresponsiveTimeout (%v), "+
			"otherwise a channel is closed between scheduled heartbeats",
			options.SendInterval, options.CloseUnresponsiveTimeout)
	}

	return nil
}
