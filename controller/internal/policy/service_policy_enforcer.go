/*
	Copyright 2019 Netfoundry, Inc.

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
	"time"

	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/runner"
	"go.etcd.io/bbolt"
)

type ServicePolicyEnforcer struct {
	appEnv *env.AppEnv
	*runner.BaseOperation
}

func NewServicePolicyEnforcer(appEnv *env.AppEnv, f time.Duration) *ServicePolicyEnforcer {
	return &ServicePolicyEnforcer{
		appEnv:        appEnv,
		BaseOperation: runner.NewBaseOperation("ServicePolicyEnforcer", f)}
}

func (enforcer *ServicePolicyEnforcer) Run() error {
	result, err := enforcer.appEnv.GetHandlers().Session.HandleQuery("")

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
			if session.IsHosting {
				policyType = persistence.PolicyTypeBind
			}
			query := fmt.Sprintf(`id = "%v" and not isEmpty(from servicePolicies where type = %v and anyOf(services) = "%v")`, identity.Id, policyType, session.ServiceId)
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
		_ = enforcer.appEnv.GetHandlers().Session.HandleDelete(sessionId)
	}

	return nil
}
