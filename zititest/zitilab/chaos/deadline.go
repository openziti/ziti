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
// that does not count time during which a component's host is unreachable. An
// infra or network outage is not a convergence failure, so a caller extends the
// deadline while the host is unreachable, up to a cap. This keeps a transient
// network blip from failing an otherwise-converging run, while still failing fast
// on real non-convergence (host reachable, state wrong) and on a permanently gone
// host (the cap is exhausted).
type ConvergenceDeadline struct {
	deadline  time.Time
	grace     time.Duration
	extended  time.Duration
	maxExtend time.Duration
}

// NewConvergenceDeadline returns a deadline that expires timeout from now and may
// be extended by up to maxExtend in total while a host is unreachable.
func NewConvergenceDeadline(timeout, maxExtend time.Duration) *ConvergenceDeadline {
	return &ConvergenceDeadline{
		deadline:  time.Now().Add(timeout),
		grace:     5 * time.Minute,
		maxExtend: maxExtend,
	}
}

// Expired reports whether the deadline has passed.
func (d *ConvergenceDeadline) Expired() bool {
	return !time.Now().Before(d.deadline)
}

// ExtendForUnreachableHost keeps the deadline at least one grace period in the
// future while the component's host is unreachable, up to maxExtend of total
// extension. Call it when a validation attempt fails to reach the component: if
// the host is reachable the failure counts against the deadline as usual; if it is
// unreachable the deadline is pushed out so the infra outage is not charged to the
// convergence budget. Returns true if it extended the deadline.
func (d *ConvergenceDeadline) ExtendForUnreachableHost(run model.Run, c *model.Component) bool {
	return d.extendUnlessReachable(HostReachable(run, c))
}

// extendUnlessReachable holds the deadline math, separated from the host probe so
// it can be exercised without a live host.
func (d *ConvergenceDeadline) extendUnlessReachable(reachable bool) bool {
	if d.extended >= d.maxExtend || reachable {
		return false
	}
	target := time.Now().Add(d.grace)
	if !target.After(d.deadline) {
		return false
	}
	add := target.Sub(d.deadline)
	if add > d.maxExtend-d.extended {
		add = d.maxExtend - d.extended
	}
	d.deadline = d.deadline.Add(add)
	d.extended += add
	return true
}
