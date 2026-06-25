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

// Test_ConvergenceDeadline_CreditsUnreachableTime verifies that the full duration
// a host is unreachable is credited back to the deadline, even for an outage that
// starts immediately in a long timeout. A 6m outage at the start of a 10m timeout
// pushes the deadline to 16m, so none of the outage is charged to convergence.
func Test_ConvergenceDeadline_CreditsUnreachableTime(t *testing.T) {
	req := require.New(t)

	start := time.Unix(0, 0)
	now := start
	d := newConvergenceDeadline(func() time.Time { return now }, 10*time.Minute, 30*time.Minute)

	// Host unreachable for the first 6 minutes, polled once a minute. Each poll
	// credits the minute since the previous one back to the deadline.
	for i := 0; i < 6; i++ {
		now = now.Add(time.Minute)
		req.True(d.Observe(false))
	}
	req.Equal(6*time.Minute, d.extended)

	// Past the original 10m timeout the run would have failed without the credit;
	// with it there is still budget left.
	now = start.Add(10*time.Minute + time.Second)
	req.False(d.Expired(), "the 6m outage must not be charged to the convergence budget")

	// Past the extended 16m deadline it finally expires.
	now = start.Add(16*time.Minute + time.Second)
	req.True(d.Expired())
}

// Test_ConvergenceDeadline_DoesNotCreditReachableTime verifies that time while the
// host is reachable is charged normally, so a reachable-but-failing component
// still expires at the original timeout.
func Test_ConvergenceDeadline_DoesNotCreditReachableTime(t *testing.T) {
	req := require.New(t)

	start := time.Unix(0, 0)
	now := start
	d := newConvergenceDeadline(func() time.Time { return now }, 10*time.Minute, 30*time.Minute)

	for i := 0; i < 6; i++ {
		now = now.Add(time.Minute)
		req.False(d.Observe(true))
	}
	req.Equal(time.Duration(0), d.extended)

	now = start.Add(10*time.Minute + time.Second)
	req.True(d.Expired(), "reachable time is charged, so it expires at the original timeout")
}

// Test_ConvergenceDeadline_ExtensionCapped verifies the credited time is capped at
// maxExtend, so a permanently gone host still eventually fails.
func Test_ConvergenceDeadline_ExtensionCapped(t *testing.T) {
	req := require.New(t)

	start := time.Unix(0, 0)
	now := start
	d := newConvergenceDeadline(func() time.Time { return now }, 10*time.Minute, 2*time.Minute)

	// A 5m unreachable gap is credited only up to the 2m cap.
	now = now.Add(5 * time.Minute)
	req.True(d.Observe(false))
	req.Equal(2*time.Minute, d.extended, "credit should be clamped to the cap")

	// Once the cap is exhausted, further unreachable time is not credited.
	now = now.Add(5 * time.Minute)
	req.False(d.Observe(false), "no credit once the cap is exhausted")
	req.Equal(2*time.Minute, d.extended)

	// Deadline is the original 10m plus the 2m cap.
	now = start.Add(12*time.Minute + time.Second)
	req.True(d.Expired())
}
