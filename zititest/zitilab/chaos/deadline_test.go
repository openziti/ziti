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

package chaos

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test_ConvergenceDeadline_ExtendsOnlyForUnreachableHost verifies the deadline is
// pushed out while the host is unreachable but not while it is reachable, so an
// infra outage is excused while a reachable-but-failing controller still expires.
func Test_ConvergenceDeadline_ExtendsOnlyForUnreachableHost(t *testing.T) {
	req := require.New(t)

	d := NewConvergenceDeadline(time.Millisecond, 10*time.Minute)
	time.Sleep(5 * time.Millisecond)
	req.True(d.Expired(), "should expire after the short timeout")

	// Reachable host: a failure does not extend the deadline, so it stays expired.
	req.False(d.extendUnlessReachable(true))
	req.True(d.Expired())

	// Unreachable host: the deadline is pushed into the future, so it is no longer
	// expired and the run keeps polling instead of failing on an infra blip.
	req.True(d.extendUnlessReachable(false))
	req.False(d.Expired(), "deadline should be extended while the host is unreachable")

	// A reachable check never extends, even after a prior extension, and the
	// deadline stays in the extended window.
	req.False(d.extendUnlessReachable(true))
	req.False(d.Expired(), "still within the extended window")
}

// Test_ConvergenceDeadline_ExtensionCapped verifies extension stops once the cap
// is consumed, so a permanently gone host still eventually fails.
func Test_ConvergenceDeadline_ExtensionCapped(t *testing.T) {
	req := require.New(t)

	// Tiny timeout and a cap smaller than one grace period, so the first extend
	// consumes the whole budget and the second cannot extend further.
	d := NewConvergenceDeadline(time.Millisecond, time.Second)
	time.Sleep(5 * time.Millisecond)

	req.True(d.extendUnlessReachable(false))
	req.Equal(d.maxExtend, d.extended, "first extension should be clamped to the cap")
	req.False(d.extendUnlessReachable(false), "no extension once the cap is exhausted")
}
