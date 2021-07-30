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

package model

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"go.etcd.io/bbolt"
	"runtime/debug"
	"time"
)

func NewPostureResponseHandler(env Env) *PostureResponseHandler {
	handler := &PostureResponseHandler{
		env:          env,
		postureCache: newPostureCache(env),
	}

	handler.AddPostureDataListener(handler.postureDataUpdated)
	return handler
}

type PostureResponseHandler struct {
	env          Env
	postureCache *PostureCache
}

func (handler *PostureResponseHandler) Create(identityId string, postureResponses []*PostureResponse) {
	handler.postureCache.Add(identityId, postureResponses)
}

// SetMfaPosture sets the MFA passing status a specific API Session owned by an identity
func (handler *PostureResponseHandler) SetMfaPosture(identityId string, apiSessionId string, isPassed bool) {
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

	handler.Create(identityId, []*PostureResponse{postureResponse})
}

// SetMfaPostureForIdentity sets the MFA passing status for all API Sessions associated to an identity
func (handler *PostureResponseHandler) SetMfaPostureForIdentity(identityId string, isPassed bool) {
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

	handler.postureCache.Upsert(identityId, true, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		var pd *PostureData

		if exist {
			pd = valueInMap.(*PostureData)
		} else {
			pd = newValue.(*PostureData)
		}

		if pd == nil {
			pfxlog.Logger().Errorf("attempting to set MFA status for identity [%s] but existing/new values was not posture data: existing [%v] new [%v]", identityId, valueInMap, newValue)
			pd = newPostureData()
		}

		for apiSessionId, _ := range pd.ApiSessions {
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

func (handler *PostureResponseHandler) AddPostureDataListener(cb func(env Env, identityId string)) {
	handler.postureCache.AddListener(EventIdentityPostureDataAltered, func(i ...interface{}) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("panic during posture data listener handler execution: %v\n", r)
				fmt.Println(string(debug.Stack()))
			}
		}()
		if identityId, ok := i[0].(string); ok {
			cb(handler.env, identityId)
		}
	})
}

// Kills active sessions that do not have passing posture checks. Run when posture data is updated
// via posture response or posture data timeout.
func (handler *PostureResponseHandler) postureDataUpdated(env Env, identityId string) {
	var sessionIdsToDelete []string

	//todo: Be smarter about this? Store a cache of service-> result on postureDataCach detect changes?
	// Only an issue when timeouts are being used - which aren't right now.
	env.HandleServiceUpdatedEventForIdentityId(identityId)

	err := handler.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		apiSessionIds, _, err := handler.env.GetStores().ApiSession.QueryIds(tx, fmt.Sprintf(`identity = "%v"`, identityId))

		if err != nil {
			return fmt.Errorf("could not query for api sessions for identity: %v", err)
		}

		for _, apiSessionId := range apiSessionIds {
			result, err := handler.env.GetHandlers().Session.Query(fmt.Sprintf(`apiSession = "%v"`, apiSessionId))

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
					policyPostureChecks := handler.env.GetHandlers().EdgeService.GetPolicyPostureChecks(identityId, session.ServiceId)

					if len(policyPostureChecks) == 0 {
						isValidService = true
					} else {
						for policyId, checks := range policyPostureChecks {
							isValidPolicy, isEvaluatedPolicy := validPolicies[policyId]

							if !isEvaluatedPolicy { //not checked yet
								validPolicies[policyId] = false
								isValidPolicy = false
								if ok, _ := handler.postureCache.Evaluate(identityId, apiSessionId, checks.PostureChecks); ok {
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
		_ = handler.env.GetHandlers().Session.Delete(sessionId)
	}

}

func (handler *PostureResponseHandler) Evaluate(identityId, apiSessionId string, check *PostureCheck) (bool, *PostureCheckFailure) {
	isValid, failures := handler.postureCache.Evaluate(identityId, apiSessionId, []*PostureCheck{check})

	if isValid {
		return true, nil
	} else {
		return false, failures[0]
	}
}

func (handler *PostureResponseHandler) PostureData(id string) *PostureData {
	return handler.postureCache.PostureData(id)
}

func (handler *PostureResponseHandler) SetSdkInfo(identityId, apiSessionId string, sdkInfo *SdkInfo) {
	if identityId == "" || apiSessionId == "" || sdkInfo == nil {
		return
	}

	handler.postureCache.Upsert(identityId, false, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		var postureData *PostureData
		if exist {
			postureData = valueInMap.(*PostureData)
		} else {
			postureData = newValue.(*PostureData)
		}

		if _, ok := postureData.ApiSessions[apiSessionId]; !ok {
			postureData.ApiSessions[apiSessionId] = &ApiSessionPostureData{}
		}

		apiSessionData := postureData.ApiSessions[apiSessionId]
		apiSessionData.SdkInfo = sdkInfo

		return postureData
	})
}
