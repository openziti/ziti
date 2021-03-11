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
	cmap "github.com/orcaman/concurrent-map"
	"regexp"
	"strings"
	"time"
)

const (
	EventIdentityPostureDataAltered   = "EventIdentityPostureDataAltered"
	EventApiSessionPostureDataAltered = "EventApiSessionPostureDataAltered"
)

type PostureCache struct {
	identityToPostureData cmap.ConcurrentMap //identityId -> PostureData
	ticker                *time.Ticker
	events.EventEmmiter
}

func newPostureCache() *PostureCache {
	pc := &PostureCache{
		identityToPostureData: cmap.New(),
		ticker:                time.NewTicker(10 * time.Second),
		EventEmmiter:          events.New(),
	}

	return pc
}

func (pc *PostureCache) Add(identityId string, postureResponses []*PostureResponse) {
	pc.identityToPostureData.Upsert(identityId, newPostureData(), func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
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

type ApiSessionPostureData struct {
	Mfa *PostureResponseMfa `json:"mfa"`
}

type PostureCheckFailureSubType interface {
	Value() interface{}
	Expected() interface{}
}

type PostureCheckFailure struct {
	PostureCheckId   string `json:"postureCheckId'"`
	PostureCheckName string `json: "postureCheckName"`
	PostureCheckType string `json: "postureCheckType"'`
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
