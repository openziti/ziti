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

package model

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/ast"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"go.etcd.io/bbolt"
	"runtime/debug"
	"time"
)

func NewPostureResponseManager(env Env) *PostureResponseManager {
	manager := &PostureResponseManager{
		env:          env,
		postureCache: newPostureCache(env),
	}

	manager.AddPostureDataListener(manager.postureDataUpdated)
	return manager
}

type PostureResponseManager struct {
	env          Env
	postureCache *PostureCache
}

func (self *PostureResponseManager) Create(identityId string, postureResponses []*PostureResponse) {
	self.postureCache.Add(identityId, postureResponses)
}

// SetMfaPosture sets the MFA passing status a specific API Session owned by an identity
func (self *PostureResponseManager) SetMfaPosture(identityId string, apiSessionId string, isPassed bool) {
	postureResponse := &PostureResponse{
		PostureCheckId: MfaProviderZiti,
		TypeId:         PostureCheckTypeMFA,
		TimedOut:       false,
		LastUpdatedAt:  time.Now().UTC(),
	}

	var passedAt *time.Time = nil

	if isPassed {
		passedAt = &postureResponse.LastUpdatedAt
	}

	postureSubType := &PostureResponseMfa{
		ApiSessionId: apiSessionId,
		PassedMfaAt:  passedAt,
	}

	postureResponse.SubType = postureSubType
	postureSubType.PostureResponse = postureResponse

	self.Create(identityId, []*PostureResponse{postureResponse})
}

// SetMfaPostureForIdentity sets the MFA passing status for all API Sessions associated to an identity
func (self *PostureResponseManager) SetMfaPostureForIdentity(identityId string, isPassed bool) {
	postureResponse := &PostureResponse{
		PostureCheckId: MfaProviderZiti,
		TypeId:         PostureCheckTypeMFA,
		TimedOut:       false,
		LastUpdatedAt:  time.Now().UTC(),
	}

	var passedAt *time.Time = nil

	if isPassed {
		passedAt = &postureResponse.LastUpdatedAt
	}

	self.postureCache.Upsert(identityId, true, func(exist bool, valueInMap *PostureData, newValue *PostureData) *PostureData {
		var pd *PostureData

		if exist {
			pd = valueInMap
		} else {
			pd = newValue
		}

		if pd == nil {
			pfxlog.Logger().Errorf("attempting to set MFA status for identity [%s] but existing/new values was not posture data: existing [%v] new [%v]", identityId, valueInMap, newValue)
			pd = newPostureData()
		}

		for apiSessionId := range pd.ApiSessions {
			postureSubType := &PostureResponseMfa{
				ApiSessionId: apiSessionId,
				PassedMfaAt:  passedAt,
			}

			postureResponse.SubType = postureSubType
			postureSubType.PostureResponse = postureResponse

			postureResponse.Apply(pd)
		}
		return pd
	})
}

func (self *PostureResponseManager) AddPostureDataListener(cb func(env Env, identityId string)) {
	self.postureCache.AddListener(EventIdentityPostureDataAltered, func(i ...interface{}) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("panic during posture data listener execution: %v\n", r)
				fmt.Println(string(debug.Stack()))
			}
		}()
		if identityId, ok := i[0].(string); ok {
			cb(self.env, identityId)
		}
	})
}

// Kills active sessions that do not have passing posture checks. Run when posture data is updated
// via posture response or posture data timeout.
func (self *PostureResponseManager) postureDataUpdated(env Env, identityId string) {
	var sessionIdsToDelete []string

	//todo: Be smarter about this? Store a cache of service-> result on postureDataCach detect changes?
	// Only an issue when timeouts are being used - which aren't right now.
	env.HandleServiceUpdatedEventForIdentityId(identityId)

	err := self.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		apiSessionIds, _, err := self.env.GetStores().ApiSession.QueryIds(tx, fmt.Sprintf(`identity = "%v"`, identityId))

		if err != nil {
			return fmt.Errorf("could not query for api sessions for identity: %v", err)
		}

		for _, apiSessionId := range apiSessionIds {
			result, err := self.env.GetManagers().Session.Query(fmt.Sprintf(`apiSession = "%v"`, apiSessionId))

			if err != nil {
				return fmt.Errorf("could not query for sessions: %v", err)
			}

			validServices := map[string]bool{} //cache services access, scoped by apiSessionId
			validPolicies := map[string]bool{} //cache policy posture check results, scoped by apiSessionId

			for _, session := range result.Sessions {
				isValidService, isEvaluatedService := validServices[session.ServiceId]

				//if we have evaluated positive access before, don't do it again
				if !isEvaluatedService {
					validServices[session.ServiceId] = false
					policyPostureChecks := self.env.GetManagers().EdgeService.GetPolicyPostureChecks(identityId, session.ServiceId)

					if len(policyPostureChecks) == 0 {
						isValidService = true
					} else {
						for policyId, checks := range policyPostureChecks {
							isValidPolicy, isEvaluatedPolicy := validPolicies[policyId]

							if !isEvaluatedPolicy { //not checked yet
								validPolicies[policyId] = false
								isValidPolicy = false
								if ok, _ := self.postureCache.Evaluate(identityId, apiSessionId, checks.PostureChecks); ok {
									isValidService = true
									isValidPolicy = true
								}
							}

							validPolicies[policyId] = isValidPolicy

							if isValidPolicy {
								break //found 1 valid policy, stop
							}
						}
					}

					if isValidService {
						validServices[session.ServiceId] = true
						break //found the service valid stop checking
					}
				}

				if !isValidService {
					sessionIdsToDelete = append(sessionIdsToDelete, session.Id)
				}
			}
		}
		return nil
	})

	if err != nil {
		pfxlog.Logger().Errorf("could not process posture data update: %v", err)
		return
	}

	for _, sessionId := range sessionIdsToDelete {
		//todo: delete batch?
		_ = self.env.GetManagers().Session.Delete(sessionId, change.New().SetSourceType("posture.cache").SetChangeAuthorType(change.AuthorTypeController))
	}
}

func (self *PostureResponseManager) Evaluate(identityId, apiSessionId string, check *PostureCheck) (bool, *PostureCheckFailure) {
	isValid, failures := self.postureCache.Evaluate(identityId, apiSessionId, []*PostureCheck{check})

	if isValid {
		return true, nil
	} else {
		return false, failures[0]
	}
}

func (self *PostureResponseManager) PostureData(id string) *PostureData {
	return self.postureCache.PostureData(id)
}

func (self *PostureResponseManager) WithPostureData(id string, f func(data *PostureData)) {
	self.postureCache.WithPostureData(id, f)
}

func (self *PostureResponseManager) SetSdkInfo(identityId, apiSessionId string, sdkInfo *SdkInfo) {
	if identityId == "" || apiSessionId == "" || sdkInfo == nil {
		return
	}

	self.postureCache.Upsert(identityId, false, func(exist bool, valueInMap *PostureData, newValue *PostureData) *PostureData {
		var postureData *PostureData
		if exist {
			postureData = valueInMap
		} else {
			postureData = newValue
		}

		if _, ok := postureData.ApiSessions[apiSessionId]; !ok {
			postureData.ApiSessions[apiSessionId] = &ApiSessionPostureData{}
		}

		apiSessionData := postureData.ApiSessions[apiSessionId]
		apiSessionData.SdkInfo = sdkInfo

		return postureData
	})
}

type ServiceWithTimeout struct {
	Service *Service
	Timeout int64
}

func shouldPostureCheckTimeoutBeAltered(mfaCheck *db.PostureCheckMfa, timeSinceLastMfa, gracePeriod time.Duration, onWake, onUnlock bool) bool {
	if mfaCheck == nil {
		return false
	}

	if (mfaCheck.PromptOnUnlock && onUnlock) || (mfaCheck.PromptOnWake && onWake) {
		//no time out the remaining time was bigger than the grace period
		timeSinceLastMfaSeconds := int64(timeSinceLastMfa.Seconds())
		gracePeriodSeconds := int64(gracePeriod.Seconds())

		timeoutShouldBeAltered := mfaCheck.TimeoutSeconds == -1 || (mfaCheck.TimeoutSeconds-timeSinceLastMfaSeconds > gracePeriodSeconds)

		return timeoutShouldBeAltered
	}

	return false
}

func (self *PostureResponseManager) GetEndpointStateChangeAffectedServices(timeSinceLastMfa, gracePeriod time.Duration, onWake bool, onUnlock bool) []*ServiceWithTimeout {
	affectedChecks := map[string]int64{} //check id -> timeout
	if onWake || onUnlock {
		queryStr := fmt.Sprintf("%s=true or %s=true", db.FieldPostureCheckMfaPromptOnUnlock, db.FieldPostureCheckMfaPromptOnWake)
		query, err := ast.Parse(self.env.GetStores().PostureCheck, queryStr)
		if err != nil {
			pfxlog.Logger().Errorf("error querying for onWake/onUnlock posture checks: %v", err)
		} else {
			err = self.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
				cursor := self.env.GetStores().PostureCheck.IterateIds(tx, query)

				for cursor.IsValid() {
					if check, err := self.env.GetStores().PostureCheck.LoadOneById(tx, string(cursor.Current())); err == nil {
						if mfaCheck, ok := check.SubType.(*db.PostureCheckMfa); ok {
							if shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, onWake, onUnlock) {
								affectedChecks[check.Id] = mfaCheck.TimeoutSeconds
							}
						}
					} else {
						pfxlog.Logger().Errorf("error querying for onWake/onUnlock posture checks by id: %v", err)
					}

					cursor.Next()
				}
				return nil
			})
			if err != nil {
				pfxlog.Logger().WithError(err).Error("error querying for onWake/onUnlock posture by id")
			}
		}
	}

	services := map[string]*ServiceWithTimeout{}

	if len(affectedChecks) > 0 {
		_ = self.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
			for checkId, timeout := range affectedChecks {
				policyCursor := self.env.GetStores().PostureCheck.GetRelatedEntitiesCursor(tx, checkId, db.EntityTypeServicePolicies, true)

				for policyCursor.IsValid() {
					serviceCursor := self.env.GetStores().ServicePolicy.GetRelatedEntitiesCursor(tx, string(policyCursor.Current()), db.EntityTypeServices, true)

					for serviceCursor.IsValid() {
						if _, ok := services[string(serviceCursor.Current())]; !ok {
							service, err := self.env.GetStores().EdgeService.LoadOneById(tx, string(serviceCursor.Current()))
							if err == nil {
								modelService := &Service{}
								if err := modelService.fillFrom(self.env, tx, service); err == nil {
									//use the lowest configured timeout (which is some timeout or no timeout)
									if existingService, ok := services[service.Id]; !ok || timeout < existingService.Timeout {
										services[service.Id] = &ServiceWithTimeout{
											Service: modelService,
											Timeout: timeout,
										}
									}
								}
							}
						}
						serviceCursor.Next()
					}

					policyCursor.Next()
				}
			}

			return nil
		})
	}

	var serviceSlice []*ServiceWithTimeout
	for _, service := range services {
		serviceSlice = append(serviceSlice, service)
	}
	return serviceSlice
}
