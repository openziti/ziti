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
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/storage/boltz"
	cmap "github.com/orcaman/concurrent-map"
	"go.etcd.io/bbolt"
	"regexp"
	"strings"
	"time"
)

const (
	EventIdentityPostureDataAltered   = "EventIdentityPostureDataAltered"
	EventApiSessionPostureDataAltered = "EventApiSessionPostureDataAltered"
)

type PostureCache struct {
	identityToPostureData    cmap.ConcurrentMap //identityId -> PostureData
	apiSessionIdToIdentityId cmap.ConcurrentMap //apiSessionId -> identityId
	ticker                   *time.Ticker
	events.EventEmmiter
	env Env
}

func newPostureCache(env Env) *PostureCache {
	pc := &PostureCache{
		identityToPostureData:    cmap.New(),
		apiSessionIdToIdentityId: cmap.New(),
		ticker:                   time.NewTicker(10 * time.Second),
		EventEmmiter:             events.New(),
		env:                      env,
	}

	pc.run(env.GetHostController().GetCloseNotifyChannel())

	env.GetStores().Session.AddListener(boltz.EventCreate, pc.SessionCreated)
	env.GetStores().Session.AddListener(boltz.EventDelete, pc.SessionDeleted)
	env.GetStores().ApiSession.AddListener(boltz.EventCreate, pc.ApiSessionCreated)
	env.GetStores().ApiSession.AddListener(boltz.EventDelete, pc.ApiSessionDeleted)
	env.GetStores().Identity.AddListener(boltz.EventDelete, pc.IdentityDeleted)

	return pc
}

func (pc *PostureCache) run(closeNotify <-chan struct{}) {
	go func() {
		for {
			select {
			case <-pc.ticker.C:
				changedIdentityIds := map[string]struct{}{}

				for _, identityId := range pc.identityToPostureData.Keys() {
					pc.identityToPostureData.Upsert(identityId, newPostureData(), func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
						var postureData *PostureData
						if exist {
							postureData = valueInMap.(*PostureData)
						} else {
							postureData = newPostureData()
						}

						if changed := postureData.CheckTimeouts(); changed {
							changedIdentityIds[identityId] = struct{}{}
						}

						return postureData
					})
				}
				for identityId := range changedIdentityIds {
					pc.Emit(EventIdentityPostureDataAltered, identityId)
				}
			case <-closeNotify:
				pc.ticker.Stop()
				return
			}
		}
	}()
}

func (pc *PostureCache) Add(identityId string, postureResponses []*PostureResponse) {
	pc.Upsert(identityId, true, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		var postureData *PostureData
		if exist {
			postureData = valueInMap.(*PostureData)
		} else {
			postureData = newValue.(*PostureData)
		}

		for _, postureResponse := range postureResponses {
			postureResponse.Apply(postureData)
		}

		return postureData
	})

	pc.Emit(EventIdentityPostureDataAltered, identityId)
}

func (pc *PostureCache) Upsert(identityId string, emitDataAltered bool, cb func(exist bool, valueInMap interface{}, newValue interface{}) interface{}) {
	pc.identityToPostureData.Upsert(identityId, newPostureData(), cb)

	if emitDataAltered {
		pc.Emit(EventIdentityPostureDataAltered, identityId)
	}
}

const MaxPostureFailures = 100

func (pc *PostureCache) AddSessionRequestFailure(identityId string, failure *PostureSessionRequestFailure) {
	pc.identityToPostureData.Upsert(identityId, newPostureData(), func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		var postureData *PostureData
		if exist {
			postureData = valueInMap.(*PostureData)
		} else {
			postureData = newValue.(*PostureData)
		}

		postureData.SessionRequestFailures = append(postureData.SessionRequestFailures, failure)

		if len(postureData.SessionRequestFailures) > MaxPostureFailures {
			postureData.SessionRequestFailures = postureData.SessionRequestFailures[1:]
		}

		return postureData
	})
}

func (pc *PostureCache) Evaluate(identityId, apiSessionId string, postureChecks []*PostureCheck) (bool, []*PostureCheckFailure) {
	if val, found := pc.identityToPostureData.Get(identityId); found {
		postureData := val.(*PostureData)
		return postureData.Evaluate(apiSessionId, postureChecks)
	}

	//mock failures with nil provided data, no posture data found
	var failures []*PostureCheckFailure
	for _, check := range postureChecks {
		failures = append(failures, &PostureCheckFailure{
			PostureCheckId:            check.Id,
			PostureCheckName:          check.Name,
			PostureCheckType:          check.TypeId,
			PostureCheckFailureValues: check.SubType.FailureValues("", newPostureData()),
		})
	}

	return false, failures
}

func (pc *PostureCache) PostureData(identityId string) *PostureData {
	if val, found := pc.identityToPostureData.Get(identityId); found {
		return val.(*PostureData)
	}

	return newPostureData()
}

func (pc *PostureCache) SessionCreated(args ...interface{}) {
	var session *persistence.Session
	if len(args) == 1 {
		session, _ = args[0].(*persistence.Session)
	}

	if session == nil {
		pfxlog.Logger().Error("session created event trigger with args[0] not convertible to *persistence.Session")
		return
	}

	mfaTimeout := int64(0)

	postureCheckLinks := pc.env.GetStores().ServicePolicy.GetLinkCollection(persistence.EntityTypePostureChecks)

	for _, policyId := range session.ServicePolicies {
		pc.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
			cursor := postureCheckLinks.IterateLinks(tx, []byte(policyId))

			for cursor.IsValid() {
				checkId := string(cursor.Current())
				if check, err := pc.env.GetStores().PostureCheck.LoadOneById(tx, checkId); err == nil {
					if check.TypeId == PostureCheckTypeMFA {
						if mfaCheck, ok := check.SubType.(*persistence.PostureCheckMfa); ok {
							if mfaCheck.TimeoutSeconds == PostureCheckNoTimeout {
								mfaTimeout = PostureCheckNoTimeout
							} else if mfaTimeout != PostureCheckNoTimeout {
								if mfaCheck.TimeoutSeconds > mfaTimeout {
									mfaTimeout = mfaCheck.TimeoutSeconds
								}
							}
						}
					}
				}
				cursor.Next()
			}
			return nil
		})
	}

	if identityIdVal, ok := pc.apiSessionIdToIdentityId.Get(session.ApiSessionId); ok {
		identityId := identityIdVal.(string)
		pc.identityToPostureData.Upsert(identityId, newPostureData(), func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
			var pd *PostureData

			if exist {
				pd = valueInMap.(*PostureData)
			} else {
				pd = newValue.(*PostureData)
			}

			if pd.ApiSessions == nil {
				pd.ApiSessions = map[string]*ApiSessionPostureData{}
			}

			if _, ok := pd.ApiSessions[session.ApiSessionId]; !ok {
				pd.ApiSessions[session.ApiSessionId] = &ApiSessionPostureData{}
			}

			if pd.ApiSessions[session.ApiSessionId].Sessions == nil {
				pd.ApiSessions[session.ApiSessionId].Sessions = map[string]*PostureSessionData{}
			}

			pd.ApiSessions[session.ApiSessionId].Sessions[session.Id] = &PostureSessionData{
				MfaTimeout: mfaTimeout,
			}

			return pd
		})
	}
}

func (pc *PostureCache) SessionDeleted(args ...interface{}) {
	var session *persistence.Session
	if len(args) == 1 {
		session, _ = args[0].(*persistence.Session)
	}

	if session == nil {
		pfxlog.Logger().Error("session deleted event trigger with args[0] not convertible to *persistence.Session")
		return
	}

	if identityIdVal, ok := pc.apiSessionIdToIdentityId.Get(session.ApiSessionId); ok {
		identityId := identityIdVal.(string)
		pc.identityToPostureData.Upsert(identityId, newPostureData(), func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
			var postureData *PostureData
			if exist {
				postureData = valueInMap.(*PostureData)
			} else {
				postureData = newValue.(*PostureData)
			}

			if apiSessionData, ok := postureData.ApiSessions[session.ApiSessionId]; ok {
				if apiSessionData.Sessions != nil {
					delete(apiSessionData.Sessions, session.Id)
				}
			}

			return postureData
		})
	}
}

func (pc *PostureCache) ApiSessionCreated(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	}

	if apiSession == nil {
		pfxlog.Logger().Error("api session create event trigger with args[0] not convertible to *persistence.ApiSession")
		return
	}

	pc.apiSessionIdToIdentityId.Set(apiSession.Id, apiSession.IdentityId)
}

func (pc *PostureCache) ApiSessionDeleted(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	}

	if apiSession == nil {
		pfxlog.Logger().Error("api session delete event trigger with args[0] not convertible to *persistence.ApiSession")
		return
	}

	pc.identityToPostureData.Upsert(apiSession.IdentityId, newPostureData(), func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		if exist {
			pd := valueInMap.(*PostureData)

			if pd != nil && pd.ApiSessions != nil {
				delete(pd.ApiSessions, apiSession.Id)
			}

			return pd
		}

		return newValue
	})

	pc.apiSessionIdToIdentityId.Remove(apiSession.Id)
}

func (pc *PostureCache) IdentityDeleted(args ...interface{}) {
	var identity *persistence.Identity
	if len(args) == 1 {
		identity, _ = args[0].(*persistence.Identity)
	}

	if identity == nil {
		pfxlog.Logger().Error("identity delete event trigger with args[0] not convertible to *persistence.ApiSession")
		return
	}

	pc.identityToPostureData.Remove(identity.Id)
}

type PostureSessionData struct {
	MfaTimeout int64
}

type ApiSessionPostureData struct {
	Mfa           *PostureResponseMfa           `json:"mfa"`
	EndpointState *PostureResponseEndpointState `json:"endpointState"`
	Sessions      map[string]*PostureSessionData
	SdkInfo       *SdkInfo
}

type PostureCheckFailureSubType interface {
	Value() interface{}
	Expected() interface{}
}

type PostureCheckFailure struct {
	PostureCheckId   string `json:"postureCheckId'"`
	PostureCheckName string `json:"postureCheckName"`
	PostureCheckType string `json:"postureCheckType"`
	PostureCheckFailureValues
}

func (self PostureCheckFailure) ToClientErrorData() interface{} {
	return map[string]interface{}{
		"id":        self.PostureCheckId,
		"typeId":    self.PostureCheckType,
		"isPassing": false,
	}
}

type PosturePolicyFailure struct {
	PolicyId   string
	PolicyName string
	Checks     []*PostureCheckFailure
}

type PostureSessionRequestFailure struct {
	When           time.Time
	ServiceId      string
	ServiceName    string
	SessionType    string
	PolicyFailures []*PosturePolicyFailure
	ApiSessionId   string
}

type PostureData struct {
	Mac                    PostureResponseMac
	Domain                 PostureResponseDomain
	Os                     PostureResponseOs
	Processes              []*PostureResponseProcess
	ProcessPathMap         map[string]*PostureResponseProcess
	ApiSessions            map[string]*ApiSessionPostureData
	SessionRequestFailures []*PostureSessionRequestFailure
}

func (pd *PostureData) Evaluate(apiSessionId string, checks []*PostureCheck) (bool, []*PostureCheckFailure) {

	var failures []*PostureCheckFailure
	for _, check := range checks {
		if isValid, failure := check.Evaluate(apiSessionId, pd); !isValid {
			failures = append(failures, failure)
		}
	}

	return len(failures) == 0, failures
}

func (pd *PostureData) CheckTimeouts() bool {
	for _, apiSessionData := range pd.ApiSessions {
		for _, sessionData := range apiSessionData.Sessions {
			if sessionData.MfaTimeout != PostureCheckNoTimeout && apiSessionData.Mfa != nil && apiSessionData.Mfa.PassedMfaAt != nil {
				expiresAt := apiSessionData.Mfa.PassedMfaAt.Add(time.Duration(sessionData.MfaTimeout) * time.Second)
				if expiresAt.Before(time.Now()) {
					return true
				}
			}
		}
	}

	return false
}

func newPostureData() *PostureData {
	ret := &PostureData{
		Mac: PostureResponseMac{
			PostureResponse: &PostureResponse{},
			Addresses:       []string{},
		},
		Domain: PostureResponseDomain{
			PostureResponse: &PostureResponse{},
			Name:            "",
		},
		Os: PostureResponseOs{
			PostureResponse: &PostureResponse{},
			Type:            "",
			Version:         "",
			Build:           "",
		},
		Processes:              []*PostureResponseProcess{},
		ProcessPathMap:         map[string]*PostureResponseProcess{},
		ApiSessions:            map[string]*ApiSessionPostureData{},
		SessionRequestFailures: []*PostureSessionRequestFailure{},
	}

	return ret
}

type PostureResponse struct {
	PostureCheckId string
	TypeId         string
	TimedOut       bool
	LastUpdatedAt  time.Time
	SubType        PostureResponseSubType
}

func (pr *PostureResponse) Apply(postureData *PostureData) {
	pr.SubType.Apply(postureData)
}

type PostureResponseSubType interface {
	Apply(postureData *PostureData)
}

var macClean = regexp.MustCompile("[^a-f0-9]+")

func CleanHexString(hexString string) string {
	return macClean.ReplaceAllString(strings.ToLower(hexString), "")
}
