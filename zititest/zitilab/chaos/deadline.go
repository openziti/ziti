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
	"time"

	"github.com/openziti/fablab/kernel/model"
)

// HostReachable reports whether a component's host can be reached, independent of
// the application process on it. It uses the host process check (over SSH), so a
// non-nil error means the host itself is unreachable (a network or infrastructure
// problem) rather than the process being down on a reachable host. Validation
// uses this to tell "the component died" (host reachable, process gone or wedged)
// apart from "the host is unreachable" (infra), which warrant different handling.
// Intended for ServerComponents.
func HostReachable(run model.Run, c *model.Component) bool {
	_, err := c.IsRunning(run)
	return err == nil
}

// ConvergenceDeadline is a deadline for a poll-until-converged validation loop
// that does not charge wall-clock time during which a component's host is
// unreachable to the convergence budget. An infra or network outage is not a
// convergence failure, so the time a host spends unreachable is credited back to
// the deadline, up to a cap. This keeps a network outage from failing an
// otherwise-converging run, while still failing fast on real non-convergence
// (host reachable, state wrong) and on a permanently gone host (the cap bounds
// total credit).
//
// The loop must record an observation once per poll while it has not yet
// converged: call ExtendForUnreachableHost when the poll failed and reachability
// is unknown (it probes the host), or Observe(true) when the poll already knows
// the host is reachable (e.g. an API call succeeded but the state is not yet
// converged). Each observation credits the time since the previous one only when
// the host is unreachable, so skipping the reachable polls would mis-attribute
// that reachable time as unreachable.
type ConvergenceDeadline struct {
	now          func() time.Time
	deadline     time.Time
	lastObserved time.Time
	extended     time.Duration
	maxExtend    time.Duration
}

// NewConvergenceDeadline returns a deadline that expires timeout from now and may
// be extended by up to maxExtend in total to cover time a host is unreachable.
func NewConvergenceDeadline(timeout, maxExtend time.Duration) *ConvergenceDeadline {
	return newConvergenceDeadline(time.Now, timeout, maxExtend)
}

// newConvergenceDeadline is NewConvergenceDeadline with an injectable clock, so
// the extension accounting can be exercised deterministically in tests.
func newConvergenceDeadline(now func() time.Time, timeout, maxExtend time.Duration) *ConvergenceDeadline {
	start := now()
	return &ConvergenceDeadline{
		now:          now,
		deadline:     start.Add(timeout),
		lastObserved: start,
		maxExtend:    maxExtend,
	}
}

// Expired reports whether the deadline has passed.
func (d *ConvergenceDeadline) Expired() bool {
	return !d.now().Before(d.deadline)
}

// ExtendForUnreachableHost probes the component's host and records the resulting
// observation via Observe. Use it on a failed poll where reachability is unknown;
// use Observe directly when the caller already knows the host is reachable.
// Returns true if it extended the deadline.
func (d *ConvergenceDeadline) ExtendForUnreachableHost(run model.Run, c *model.Component) bool {
	return d.Observe(HostReachable(run, c))
}

// Observe records a poll observation with known reachability: it measures the time
// since the previous observation and, when reachable is false, credits that time
// to the deadline (up to maxExtend of total credit) so an outage is not charged to
// the convergence budget. When reachable is true it only advances the internal
// clock. Returns true if it extended the deadline.
func (d *ConvergenceDeadline) Observe(reachable bool) bool {
	now := d.now()
	elapsed := now.Sub(d.lastObserved)
	d.lastObserved = now
	if reachable || elapsed <= 0 || d.extended >= d.maxExtend {
		return false
	}
	if elapsed > d.maxExtend-d.extended {
		elapsed = d.maxExtend - d.extended
	}
	d.deadline = d.deadline.Add(elapsed)
	d.extended += elapsed
	return true
}
