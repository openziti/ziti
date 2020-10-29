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
)

func NewPostureResponseHandler(env Env) *PostureResponseHandler {
	handler := &PostureResponseHandler{
		env:          env,
		postureCache: newPostureCache(),
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

func (handler *PostureResponseHandler) AddPostureDataListener(cb func(identityId string)) {
	handler.postureCache.AddListener(EventIdentityPostureDataAltered, func(i ...interface{}) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("panic during posture data listener handler execution: %v\n", r)
				fmt.Println(string(debug.Stack()))
			}
		}()
		if identityId, ok := i[0].(string); ok {
			cb(identityId)
		}
	})
}

// Kills active sessions that do not have passing posture checks. Run when posture data is updated
// via posture response or posture data timeout.
func (handler *PostureResponseHandler) postureDataUpdated(identityId string) {
	var sessionIdsToDelete []string

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

			validServices := map[string]bool{} //cache services access
			validPolicies := map[string]bool{} //cache policy posture check results

			for _, session := range result.Sessions {
				isValidService, isEvaluatedService := validServices[session.ServiceId]

				//if we have evaluated positive access before, don't do it again
				if !isEvaluatedService {
					validServices[session.ServiceId] = false
					checkMap := handler.env.GetHandlers().EdgeService.GetPostureChecks(identityId, session.ServiceId)

					for policyId, checks := range checkMap {
						isValidPolicy, isEvaluatedPolicy := validPolicies[policyId]

						if !isEvaluatedPolicy { //not checked yet
							validPolicies[policyId] = false
							isValidPolicy = false
							if handler.postureCache.Evaluate(identityId, checks) {
								isValidService = true
								isValidPolicy = true
							}
						}

						validPolicies[policyId] = isValidPolicy

						if isValidPolicy {
							break //found 1 valid policy, stop
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

func (handler *PostureResponseHandler) Evaluate(identityId string, check *PostureCheck) bool {
	return handler.postureCache.Evaluate(identityId, []*PostureCheck{check})
}

func (handler *PostureResponseHandler) PostureData(id string) *PostureData {
	return handler.postureCache.PostureData(id)
}
