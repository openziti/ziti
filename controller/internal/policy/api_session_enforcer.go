/*
	Copyright NetFoundry, Inc.

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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/runner"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	maxIterations         = 1000
	maxDeletePerIteration = 500

	ApiSessionEnforcerRun    = "api.session.enforcer.run"
	ApiSessionEnforcerDelete = "api.session.enforcer.delete"
)

type ApiSessionEnforcer struct {
	appEnv         model.Env
	sessionTimeout time.Duration
	*runner.BaseOperation
}

func NewSessionEnforcer(appEnv *env.AppEnv, frequency time.Duration, sessionTimeout time.Duration) *ApiSessionEnforcer {
	if sessionTimeout < 60*time.Second {
		pfxlog.Logger().Panic("sessionTimeout can not be less than 60 seconds")
	}

	pfxlog.Logger().
		WithField("sessionTimeout", sessionTimeout.String()).
		WithField("frequency", frequency.String()).
		Info("session enforcer configured")

	return &ApiSessionEnforcer{
		appEnv:         appEnv,
		sessionTimeout: sessionTimeout,
		BaseOperation:  runner.NewBaseOperation("ApiSessionEnforcer", frequency),
	}
}

func (s *ApiSessionEnforcer) Run() error {
	startTime := time.Now()

	defer func() {
		s.appEnv.GetMetricsRegistry().Timer(ApiSessionEnforcerRun).UpdateSince(startTime)
	}()

	oldest := time.Now().Add(s.sessionTimeout * -1)
	query := fmt.Sprintf("lastActivityAt < datetime(%s) limit %d", oldest.UTC().Format(time.RFC3339), maxDeletePerIteration)

	log := pfxlog.Logger()
	for i := 0; i < maxIterations; i++ {
		ids := make([]string, 0, maxDeletePerIteration)

		//iterate over API Sessions that do not have a recent enough lastAccessedAt
		err := s.appEnv.GetHandlers().ApiSession.StreamIds(query, func(id string, err error) error {
			if lastActivityAt, ok := s.appEnv.GetHandlers().ApiSession.HeartbeatCollector.LastAccessedAt(id); ok && lastActivityAt != nil {
				log.Tracef("during API session enforcement lastAccessedAt check, API Session [%s] was found in the cache with time [%s]", id, lastActivityAt.String())
				if lastActivityAt.Before(oldest) {
					ids = append(ids, id)
				}
			} else {
				log.Tracef("during API session enforcement lastAccessedAt check, API Session [%s] was not in the cache, using lastAccessedAt from db", id)
				ids = append(ids, id)
			}

			return nil
		})

		if err != nil {
			pfxlog.Logger().Errorf("encountered error querying for API sessions to remove: %v", err)
			break
		}

		if len(ids) == 0 {
			break
		}

		logrus.Debugf("found %v expired api-sessions to remove", len(ids))

		if err = s.appEnv.GetHandlers().ApiSession.DeleteBatch(ids); err != nil {
			logrus.WithError(err).Error("failure while batch deleting expired api sessions")

			for _, id := range ids {
				if err = s.appEnv.GetHandlers().ApiSession.Delete(id); err != nil {
					logrus.WithError(err).Errorf("failure while deleting expired api session: %v", id)
				}
			}
			s.appEnv.GetMetricsRegistry().Meter(ApiSessionEnforcerDelete).Mark(int64(len(ids)))
		}
	}

	return nil
}
