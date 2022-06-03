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
	"bytes"
	"github.com/jinzhu/copier"
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/storage/ast"
	"github.com/openziti/storage/boltz"
	cmap "github.com/orcaman/concurrent-map/v2"
	"go.etcd.io/bbolt"
	"regexp"
	"strings"
	"time"
)

const (
	EventIdentityPostureDataAltered = "EventIdentityPostureDataAltered"
)

type PostureCache struct {
	identityToPostureData    cmap.ConcurrentMap[*PostureData]
	apiSessionIdToIdentityId cmap.ConcurrentMap[string]
	ticker                   *time.Ticker
	isRunning                concurrenz.AtomicBoolean
	events.EventEmmiter
	env Env
}

func newPostureCache(env Env) *PostureCache {
	pc := &PostureCache{
		identityToPostureData:    cmap.New[*PostureData](),
		apiSessionIdToIdentityId: cmap.New[string](),
		ticker:                   time.NewTicker(5 * time.Second),
		EventEmmiter:             events.New(),
		env:                      env,
		isRunning:                concurrenz.AtomicBoolean(0),
	}

	pc.run(env.GetHostController().GetCloseNotifyChannel())

	env.GetStores().ApiSession.AddListener(boltz.EventCreate, pc.ApiSessionCreated)
	env.GetStores().ApiSession.AddListener(boltz.EventDelete, pc.ApiSessionDeleted)
	env.GetStores().Identity.AddListener(boltz.EventDelete, pc.IdentityDeleted)

	env.GetStores().PostureCheckType.AddListener(boltz.EventCreate, pc.PostureCheckChanged)
	env.GetStores().PostureCheckType.AddListener(boltz.EventUpdate, pc.PostureCheckChanged)

	return pc
}

func (pc *PostureCache) run(closeNotify <-chan struct{}) {
	go func() {
		for {
			select {
			case <-pc.ticker.C:
				go pc.evaluate()
			case <-closeNotify:
				pc.ticker.Stop()
				return
			}
		}
	}()
}

func (pc *PostureCache) evaluate() {
	if !pc.isRunning.CompareAndSwap(false, true) {
		return
	}

	log := pfxlog.Logger()

	defer func() {
		if r := recover(); r != nil {
			log.Errorf("error during posture timeout enforcement: %v", r)
		}

		pc.isRunning.Set(false)
	}()

	var lastId []byte
	const maxScanPerTx = 1000
	var toDeleteSessionIds []string

	newIdentityServiceUpdates := map[string]struct{}{}       //tracks the current loops identityIds that have had updates
	completedIdentityServiceUpdates := map[string]struct{}{} //tracks overall identityIds that have been notified of an update

	done := false

	// Chunk data in maxToDelete bunches to limit how many sessions we are deleting in a transaction.
	// Requires tracking of which session was last evaluated, kept in lastId.
	for !done {
		var sessions []*persistence.Session
		_ = pc.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
			cursor := pc.env.GetStores().Session.IterateIds(tx, ast.BoolNodeTrue)

			if len(lastId) != 0 {
				cursor.Seek(lastId)

				if cursor.IsValid() {
					if bytes.Compare(cursor.Current(), lastId) == 0 {
						cursor.Next()
					}
				}
			}

			for cursor.IsValid() && len(sessions) < maxScanPerTx {
				if session, _ := pc.env.GetStores().Session.LoadOneById(tx, string(cursor.Current())); session != nil {
					sessions = append(sessions, session)
				}
				lastId = cursor.Current()
				cursor.Next()
			}

			if !cursor.IsValid() {
				done = true
			}

			return nil
		})

		for _, session := range sessions {
			result := pc.env.GetManagers().Session.EvaluatePostureForService(session.IdentityId, session.ApiSessionId, session.Type, session.ServiceId, "")

			if !result.Passed {
				log.WithFields(map[string]interface{}{
					"apiSessionId": session.ApiSessionId,
					"identityId":   session.IdentityId,
					"sessionId":    session.Id,
				}).Tracef("session [%s] failed posture checks, removing", session.Id)

				toDeleteSessionIds = append(toDeleteSessionIds, session.Id)
				newIdentityServiceUpdates[session.IdentityId] = struct{}{}
			}
		}

		//delete sessions that failed pc checks, clear list
		for _, sessionId := range toDeleteSessionIds {
			err := pc.env.GetManagers().Session.Delete(sessionId)
			if err != nil {
				log.WithError(err).Errorf("error removing session [%s] due to posture check failure, delete error: %v", sessionId, err)
			}
		}
		toDeleteSessionIds = []string{}

		//notify endpoints that they may have new service posture queries
		for identityId := range newIdentityServiceUpdates {
			if _, ok := completedIdentityServiceUpdates[identityId]; !ok {
				completedIdentityServiceUpdates[identityId] = struct{}{}
				pc.env.HandleServiceUpdatedEventForIdentityId(identityId)
			}
		}

		newIdentityServiceUpdates = map[string]struct{}{}
	}
}

func (pc *PostureCache) Add(identityId string, postureResponses []*PostureResponse) {
	pc.Upsert(identityId, true, func(exist bool, valueInMap *PostureData, newValue *PostureData) *PostureData {
		var postureData *PostureData

		if exist {
			postureData = valueInMap
		} else {
			postureData = newValue
		}

		for _, postureResponse := range postureResponses {
			postureResponse.Apply(postureData)
		}

		return postureData
	})

	pc.Emit(EventIdentityPostureDataAltered, identityId)
}

// Upsert is a convenience function to alter the existing PostureData for an identity. If
// emitDataAltered is true, posture data listeners will be alerted: this will trigger
// service update notifications and posture check evaluation.
func (pc *PostureCache) Upsert(identityId string, emitDataAltered bool, cb func(exist bool, valueInMap *PostureData, newValue *PostureData) *PostureData) {
	pc.identityToPostureData.Upsert(identityId, newPostureData(), cb)

	if emitDataAltered {
		pc.Emit(EventIdentityPostureDataAltered, identityId)
	}
}

const MaxPostureFailures = 100

func (pc *PostureCache) AddSessionRequestFailure(identityId string, failure *PostureSessionRequestFailure) {
	pc.identityToPostureData.Upsert(identityId, newPostureData(), func(exist bool, valueInMap *PostureData, newValue *PostureData) *PostureData {
		var postureData *PostureData

		if exist {
			postureData = valueInMap
		} else {
			postureData = newValue
		}

		postureData.SessionRequestFailures = append(postureData.SessionRequestFailures, failure)

		if len(postureData.SessionRequestFailures) > MaxPostureFailures {
			postureData.SessionRequestFailures = postureData.SessionRequestFailures[1:]
		}

		return postureData
	})
}

func (pc *PostureCache) Evaluate(identityId, apiSessionId string, postureChecks []*PostureCheck) (bool, []*PostureCheckFailure) {
	if postureData, found := pc.identityToPostureData.Get(identityId); found {
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

// PostureData returns a copy of the current posture data for an identity.
// Suitable for read only rendering. To alter/update posture data see Upsert.
func (pc *PostureCache) PostureData(identityId string) *PostureData {
	var result *PostureData = nil

	pc.Upsert(identityId, false, func(exist bool, valueInMap *PostureData, newValue *PostureData) *PostureData {
		var pd *PostureData
		if exist {
			pd = valueInMap
		} else {
			pd = newValue
		}

		result = pd.Copy()
		return pd
	})

	return result
}

func (pc *PostureCache) ApiSessionCreated(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	} else {
		pfxlog.Logger().Errorf("unexpected number of args [%d]", len(args))
		return
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
	} else {
		pfxlog.Logger().Errorf("unexpected number of args [%d]", len(args))
		return
	}

	if apiSession == nil {
		pfxlog.Logger().Error("api session delete event trigger with args[0] not convertible to *persistence.ApiSession")
		return
	}

	pc.identityToPostureData.Upsert(apiSession.IdentityId, newPostureData(), func(exist bool, valueInMap *PostureData, newValue *PostureData) *PostureData {
		if exist {
			if valueInMap != nil && valueInMap.ApiSessions != nil {
				delete(valueInMap.ApiSessions, apiSession.Id)
			}

			return valueInMap
		}

		return newValue
	})

	pc.apiSessionIdToIdentityId.Remove(apiSession.Id)
}

func (pc *PostureCache) IdentityDeleted(args ...interface{}) {
	var identity *persistence.Identity
	if len(args) == 1 {
		identity, _ = args[0].(*persistence.Identity)
	} else {
		pfxlog.Logger().Errorf("unexpected number of args [%d]", len(args))
		return
	}

	if identity == nil {
		pfxlog.Logger().Error("identity delete event trigger with args[0] not convertible to *persistence.ApiSession")
		return
	}

	pc.identityToPostureData.Remove(identity.Id)
}

//PostureCheckChanged notifies all associated identities that posture configuration has changed
//and that endpoints may need to reevaluate posture queries.
func (pc *PostureCache) PostureCheckChanged(args ...interface{}) {
	var entity boltz.Entity
	if len(args) == 1 {
		var ok bool
		entity, ok = args[0].(boltz.Entity)

		if !ok {
			pfxlog.Logger().Errorf("unexpected type [%T] expected boltz.Entity: %v", args[0], args[0])
		}
	} else {
		pfxlog.Logger().Errorf("unexpected number of args [%d]", len(args))
		return
	}

	servicePolicyLinks := pc.env.GetStores().PostureCheck.GetLinkCollection(persistence.EntityTypeServicePolicies)

	if servicePolicyLinks == nil {
		pfxlog.Logger().Error("posture checks had no links to service policies")
		return
	}

	identitiesToNotify := map[string]struct{}{}

	_ = pc.env.GetDbProvider().GetDb().View(func(tx *bbolt.Tx) error {
		servicePolicyCursor := servicePolicyLinks.IterateLinks(tx, []byte(entity.GetId()))

		for servicePolicyCursor.IsValid() {
			identityLink := pc.env.GetStores().ServicePolicy.GetLinkCollection(persistence.EntityTypeIdentities)

			if identityLink == nil {
				pfxlog.Logger().Error("service policies had no link to identities")
				return nil
			}

			identityCursor := identityLink.IterateLinks(tx, servicePolicyCursor.Current())

			for identityCursor.IsValid() {
				identitiesToNotify[string(identityCursor.Current())] = struct{}{}
			}

			servicePolicyCursor.Next()
		}

		return nil
	})

	for identityId := range identitiesToNotify {
		pc.env.HandleServiceUpdatedEventForIdentityId(identityId)
	}
}

type PostureSessionData struct {
	MfaTimeout int64
}

type ApiSessionPostureData struct {
	Mfa           *PostureResponseMfa           `json:"mfa"`
	EndpointState *PostureResponseEndpointState `json:"endpointState"`
	SdkInfo       *SdkInfo
}

func (self *ApiSessionPostureData) GetPassedMfaAt() *time.Time {
	if self == nil || self.Mfa == nil {
		return nil
	}
	return self.Mfa.PassedMfaAt
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

func (pd *PostureData) Copy() *PostureData {
	dest := &PostureData{}
	_ = copier.Copy(dest, pd)
	return dest
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

var macClean = regexp.MustCompile("[^a-f\\d]+")

func CleanHexString(hexString string) string {
	return macClean.ReplaceAllString(strings.ToLower(hexString), "")
}
