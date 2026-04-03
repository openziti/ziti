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
	"github.com/openziti/ziti/common/runner"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/env"
)

const (
	RevocationEnforcerRun    = "revocation.enforcer.run"
	RevocationEnforcerDelete = "revocation.enforcer.delete"
	RevocationEnforcerSource = "revocation.enforcer"
)

// RevocationEnforcer periodically purges expired revocation records from the
// controller database and router data models.
type RevocationEnforcer struct {
	appEnv *env.AppEnv
	*runner.BaseOperation
}

// NewRevocationEnforcer creates a RevocationEnforcer that runs at the given frequency.
func NewRevocationEnforcer(appEnv *env.AppEnv, frequency time.Duration) *RevocationEnforcer {
	return &RevocationEnforcer{
		appEnv:        appEnv,
		BaseOperation: runner.NewBaseOperation("RevocationEnforcer", frequency),
	}
}

// Run deletes all expired revocation entries.
func (e *RevocationEnforcer) Run() error {
	startTime := time.Now()

	defer func() {
		e.appEnv.GetMetricsRegistry().Timer(RevocationEnforcerRun).UpdateSince(startTime)
	}()

	ctx := change.New().SetSourceType(RevocationEnforcerSource).SetChangeAuthorType(change.AuthorTypeController)

	total, err := e.appEnv.GetManagers().Revocation.DeleteExpired(ctx)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to delete expired revocations")
		return nil
	}

	if total > 0 {
		pfxlog.Logger().Debugf("removed %d expired revocations", total)
		e.appEnv.GetMetricsRegistry().Meter(RevocationEnforcerDelete).Mark(int64(total))
	}

	return nil
}
