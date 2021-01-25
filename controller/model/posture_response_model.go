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

func (pc *PostureCache) Evaluate(identityId, apiSessionId string, postureChecks []*PostureCheck) bool {
	if val, found := pc.identityToPostureData.Get(identityId); found {
		postureData := val.(*PostureData)
		return postureData.Evaluate(apiSessionId, postureChecks)
	}

	return false
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

type PostureData struct {
	Mac         *PostureResponseMac               `json:"mac"`
	Domain      *PostureResponseDomain            `json:"domain"`
	Os          *PostureResponseOs                `json:"os"`
	Processes   []*PostureResponseProcess         `json:"process"`
	ApiSessions map[string]*ApiSessionPostureData `json:"sessionPostureData"`
}

func (pd *PostureData) Evaluate(apiSessionId string, checks []*PostureCheck) bool {
	for _, check := range checks {
		if result := check.Evaluate(apiSessionId, pd); !result {
			return false
		}
	}

	return true
}

func newPostureData() *PostureData {
	return &PostureData{
		Mac: &PostureResponseMac{
			PostureResponse: &PostureResponse{},
			Addresses:       []string{},
		},
		Domain: &PostureResponseDomain{
			PostureResponse: &PostureResponse{},
			Name:            "",
		},
		Os: &PostureResponseOs{
			PostureResponse: &PostureResponse{},
			Type:            "",
			Version:         "",
			Build:           "",
		},
		Processes:   []*PostureResponseProcess{},
		ApiSessions: map[string]*ApiSessionPostureData{},
	}
}

type PostureResponse struct {
	PostureCheckId string                 `json:"postureCheckId"`
	TypeId         string                 `json:"-"`
	TimedOut       bool                   `json:"timedOut"`
	LastUpdatedAt  time.Time              `json:"lastUpdatedAt"`
	SubType        PostureResponseSubType `json:"-"`
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
