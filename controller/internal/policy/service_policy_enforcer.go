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
	"time"

	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/runner"
	"go.etcd.io/bbolt"
)

const (
	SessionPolicyEnforcerRun          = "service.policy.enforcer.run"
	SessionPolicyEnforcerEvent        = "service.policy.enforcer.event"
	SessionPolicyEnforcerEventDeletes = "service.policy.enforcer.event.deletes"
	SessionPolicyEnforcerRunDeletes   = "service.policy.enforcer.run.deletes"
)

type ServicePolicyEnforcer struct {
	appEnv *env.AppEnv
	*runner.BaseOperation
	notify chan struct{}
}

func NewServicePolicyEnforcer(appEnv *env.AppEnv, f time.Duration) *ServicePolicyEnforcer {
	result := &ServicePolicyEnforcer{
		appEnv:        appEnv,
		BaseOperation: runner.NewBaseOperation("ServicePolicyEnforcer", f),
		notify:        make(chan struct{}, 1),
	}
	result.notify <- struct{}{} // ensure we do a full scan on startup
	persistence.ServiceEvents.AddServiceEventHandler(result.handleServiceEvent)
	return result
}

func (enforcer *ServicePolicyEnforcer) handleServiceEvent(event *persistence.ServiceEvent) {
	policyType := ""

	if event.Type == persistence.ServiceDialAccessLost {
		policyType = persistence.PolicyTypeDialName
	}

	if event.Type == persistence.ServiceBindAccessLost {
		policyType = persistence.PolicyTypeBindName
	}

	if policyType == "" {
		return
	}

	startTime := time.Now()
	defer func() {
		enforcer.appEnv.GetMetricsRegistry().Timer(SessionPolicyEnforcerEvent).UpdateSince(startTime)
	}()

	log := pfxlog.Logger().WithField("event", event.String())
	log.Debug("event received")

	var sessionsToDelete []string

	err := enforcer.appEnv.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		identity := &persistence.Identity{}

		if _, err := enforcer.appEnv.GetStores().Identity.BaseLoadOneById(tx, event.IdentityId, identity); err != nil {
			return err
		}

		if identity.IsAdmin {
			return nil
		}

		query := fmt.Sprintf(`apiSession.identity="%v" and service="%v" and type="%v"`, event.IdentityId, event.ServiceId, policyType)
		sessionsToDelete, _, err = enforcer.appEnv.GetStores().Session.QueryIds(tx, query)
		return err
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("error while processing event: %v", event)

		// notify enforcer that it should run on the next cycle
		select {
		case enforcer.notify <- struct{}{}:
		default:
		}
	}

	for _, sessionId := range sessionsToDelete {
		_ = enforcer.appEnv.GetManagers().Session.Delete(sessionId)
		log.Debugf("session %v deleted", sessionId)
	}

	enforcer.appEnv.GetMetricsRegistry().Meter(SessionPolicyEnforcerEventDeletes).Mark(int64(len(sessionsToDelete)))
}

func (enforcer *ServicePolicyEnforcer) Run() error {
	// if we haven't been notified to run b/c of startup or handler error, skip run
	select {
	case <-enforcer.notify:
	default:
		return nil
	}

	startTime := time.Now()

	defer func() {
		enforcer.appEnv.GetMetricsRegistry().Timer(SessionPolicyEnforcerRun).UpdateSince(startTime)
	}()

	result, err := enforcer.appEnv.GetManagers().Session.Query("")

	if err != nil {
		return err
	}

	var sessionsToRemove []string
	err = enforcer.appEnv.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		for _, session := range result.Sessions {
			apiSession := &persistence.ApiSession{}
			_, err := enforcer.appEnv.GetStores().ApiSession.BaseLoadOneById(tx, session.ApiSessionId, apiSession)

			if err != nil {
				return err
			}

			identity := &persistence.Identity{}
			_, err = enforcer.appEnv.GetStores().Identity.BaseLoadOneById(tx, apiSession.IdentityId, identity)

			if err != nil {
				return err
			}

			if identity.IsAdmin {
				continue
			}

			policyType := persistence.PolicyTypeDial
			if session.Type == persistence.SessionTypeBind {
				policyType = persistence.PolicyTypeBind
			}
			query := fmt.Sprintf(`id = "%v" and not isEmpty(from servicePolicies where type = %v and anyOf(services) = "%v")`, identity.Id, int32(policyType), session.ServiceId)
			_, count, err := enforcer.appEnv.GetStores().Identity.QueryIds(tx, query)
			if err != nil {
				return err
			}
			if count == 0 {
				sessionsToRemove = append(sessionsToRemove, session.Id)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	for _, sessionId := range sessionsToRemove {
		_ = enforcer.appEnv.GetManagers().Session.Delete(sessionId)
	}

	enforcer.appEnv.GetMetricsRegistry().Meter(SessionPolicyEnforcerRunDeletes).Mark(int64(len(sessionsToRemove)))

	return nil
}
