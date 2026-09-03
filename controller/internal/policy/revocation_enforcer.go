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

package policy

import (
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/runner"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/env"
)

const (
	RevocationEnforcerRun    = "revocation.enforcer.run"
	RevocationEnforcerDelete = "revocation.enforcer.delete"
	RevocationEnforcerSource = "revocation.enforcer"
)

// RevocationEnforcer periodically purges expired revocation records from the
// controller database and router data models. It only runs on the raft leader
// (or in a leaderless configuration) to avoid redundant batch-delete dispatches.
type RevocationEnforcer struct {
	appEnv     *env.AppEnv
	dispatcher command.Dispatcher
	// Resolved once and held. A Meter lookup takes a reference on every call and only Dispose releases
	// one, so looking one up per run accumulates references and leaves the meter sampling after the
	// registry is disposed. Timer is not reference counted, and is held here so the two read alike.
	runTimer    metrics.Timer
	deleteMeter metrics.Meter
	*runner.BaseOperation
}

// NewRevocationEnforcer creates a RevocationEnforcer that runs at the given frequency.
func NewRevocationEnforcer(appEnv *env.AppEnv, frequency time.Duration, dispatcher command.Dispatcher) *RevocationEnforcer {
	return &RevocationEnforcer{
		appEnv:        appEnv,
		dispatcher:    dispatcher,
		runTimer:      appEnv.GetMetricsRegistry().Timer(RevocationEnforcerRun),
		deleteMeter:   appEnv.GetMetricsRegistry().Meter(RevocationEnforcerDelete),
		BaseOperation: runner.NewBaseOperation("RevocationEnforcer", frequency),
	}
}

// Run deletes all expired revocation entries. It is a no-op on non-leader nodes.
func (e *RevocationEnforcer) Run() error {
	if !e.dispatcher.IsLeaderOrLeaderless() {
		return nil
	}

	startTime := time.Now()

	defer func() {
		e.runTimer.UpdateSince(startTime)
	}()

	ctx := change.New().SetSourceType(RevocationEnforcerSource).SetChangeAuthorType(change.AuthorTypeController)

	total, err := e.appEnv.GetManagers().Revocation.DeleteExpired(ctx)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to delete expired revocations")
		return nil
	}

	if total > 0 {
		pfxlog.Logger().Debugf("removed %d expired revocations", total)
		e.deleteMeter.Mark(int64(total))
	}

	return nil
}
