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

package handler_link

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test_linkHeartbeatThresholds_ShortTimeoutStaysReachable is the central case. The warning threshold used
// to gate the close check, so any close timeout at or below it could never fire: an operator asking for
// faster link failure detection silently got the warning threshold instead.
func Test_linkHeartbeatThresholds_ShortTimeoutStaysReachable(t *testing.T) {
	req := require.New(t)

	for _, closeTimeout := range []time.Duration{
		time.Second, 5 * time.Second, linkUnhealthyWarnThreshold - time.Millisecond,
		linkUnhealthyWarnThreshold, linkUnhealthyWarnThreshold + time.Second,
		time.Minute, 5 * time.Minute,
	} {
		warnAfter, closeAfter := linkHeartbeatThresholds(closeTimeout)

		req.Equal(closeTimeout, closeAfter, "close timeout %v must be honoured as given", closeTimeout)
		req.Less(warnAfter, closeAfter,
			"close timeout %v must be reachable, so the warning has to come first", closeTimeout)
		req.Greater(warnAfter, time.Duration(0),
			"close timeout %v must still warn before closing", closeTimeout)
	}
}

// Test_linkHeartbeatThresholds_DefaultsUnchanged pins the shipped configuration to what it did before the
// thresholds were derived: warn at 30s, close at 60s.
func Test_linkHeartbeatThresholds_DefaultsUnchanged(t *testing.T) {
	req := require.New(t)

	warnAfter, closeAfter := linkHeartbeatThresholds(time.Minute)
	req.Equal(30*time.Second, warnAfter)
	req.Equal(time.Minute, closeAfter)
}

// Test_linkHeartbeatThresholds_ZeroDisablesClosingButNotWarning: zero reads as "do not close", matching how
// router/env/ctrl.go treats the same option. The warning is what poisons the latency metric, which is how
// smart routing avoids the link, so it must survive closing being off.
func Test_linkHeartbeatThresholds_ZeroDisablesClosingButNotWarning(t *testing.T) {
	req := require.New(t)

	for _, closeTimeout := range []time.Duration{0, -time.Second} {
		warnAfter, closeAfter := linkHeartbeatThresholds(closeTimeout)
		req.Zero(closeAfter, "close timeout %v must disable closing", closeTimeout)
		req.Equal(linkUnhealthyWarnThreshold, warnAfter,
			"close timeout %v must still warn", closeTimeout)
	}
}
