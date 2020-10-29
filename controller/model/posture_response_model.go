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
	EventIdentityPostureDataAltered = "EventIdentityPostureDataAltered"
)

type PostureCache struct {
	identityToPostureData cmap.ConcurrentMap //identityId -> PostureData
	ticker                *time.Ticker
	postureDataTimeout    time.Duration
	events.EventEmmiter
}

func newPostureCache() *PostureCache {
	pc := &PostureCache{
		identityToPostureData: cmap.New(),
		ticker:                time.NewTicker(10 * time.Second),
		EventEmmiter:          events.New(),
		postureDataTimeout:    30 * time.Second,
	}

	pc.start()

	return pc
}

func (pc *PostureCache) start() {
	go func() {
		for range pc.ticker.C {

			changedIdentityIds := map[string]struct{}{}

			for _, identityId := range pc.identityToPostureData.Keys() {
				pc.identityToPostureData.Upsert(identityId, newPostureData(), func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
					var postureData *PostureData
					if exist {
						postureData = valueInMap.(*PostureData)
					} else {
						postureData = newPostureData()
					}

					if changed := postureData.Timeout(time.Now().Add(-1 * pc.postureDataTimeout)); changed {
						changedIdentityIds[identityId] = struct{}{}
					}

					return postureData
				})
			}
			for identityId := range changedIdentityIds {
				pc.Emit(EventIdentityPostureDataAltered, identityId)
			}
		}
	}()
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

func (pc *PostureCache) Evaluate(identityId string, postureChecks []*PostureCheck) bool {
	if val, found := pc.identityToPostureData.Get(identityId); found {
		postureData := val.(*PostureData)
		return postureData.Evaluate(postureChecks)
	}

	return false
}

func (pc *PostureCache) PostureData(identityId string) *PostureData {
	if val, found := pc.identityToPostureData.Get(identityId); found {
		return val.(*PostureData)
	}

	return newPostureData()
}

type PostureData struct {
	Mac       *PostureResponseMac       `json:"mac"`
	Domain    *PostureResponseDomain    `json:"domain"`
	Os        *PostureResponseOs        `json:"os"`
	Processes []*PostureResponseProcess `json:"process"`
}

func (pd *PostureData) Timeout(oldest time.Time) bool {
	changed := false
	changed = pd.Mac.Timeout(oldest) || changed
	changed = pd.Domain.Timeout(oldest) || changed
	changed = pd.Os.Timeout(oldest) || changed

	for _, process := range pd.Processes {
		changed = process.Timeout(oldest) || changed
	}

	return changed
}

func (pd *PostureData) Evaluate(checks []*PostureCheck) bool {
	for _, check := range checks {
		if result := check.Evaluate(pd); !result {
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
		Processes: []*PostureResponseProcess{},
	}
}

type PostureResponse struct {
	PostureCheckId string                 `json:"postureCheckId"`
	TypeId         string                 `json:"-"`
	TimedOut       bool                   `json:"timedOut"`
	LastUpdatedAt  time.Time              `json:"lastUpdatedAt"`
	SubType        PostureResponseSubType `json:"-"`
}

func (pr *PostureResponse) Timeout(oldest time.Time) bool {
	if !pr.TimedOut && pr.LastUpdatedAt.Before(oldest) {
		pr.TimedOut = true
		return true
	}

	return false
}

func (pr *PostureResponse) Apply(postureData *PostureData) {
	pr.SubType.Apply(postureData)
}

type PostureResponseSubType interface {
	Apply(postureData *PostureData)
	Timeout(oldest time.Time) bool
}

type PostureResponseMac struct {
	*PostureResponse
	Addresses []string `json:"addresses"`
}

func (pr *PostureResponseMac) Apply(postureData *PostureData) {
	var cleanedAddresses []string
	for _, address := range pr.Addresses {
		cleanedAddresses = append(cleanedAddresses, CleanHexString(address))
	}

	pr.Addresses = cleanedAddresses

	postureData.Mac = pr
	postureData.Mac.LastUpdatedAt = time.Now()
}

var macClean = regexp.MustCompile("[^a-f0-9]+")

func CleanHexString(hexString string) string {
	return macClean.ReplaceAllString(strings.ToLower(hexString), "")
}

type PostureResponseOs struct {
	*PostureResponse
	Type    string `json:"type"`
	Version string `json:"version"`
	Build   string `json:"build"`
}

func (pr *PostureResponseOs) Apply(postureData *PostureData) {
	postureData.Os = pr
	postureData.Os.LastUpdatedAt = time.Now()
}

type PostureResponseProcess struct {
	*PostureResponse
	IsRunning         bool   `json:"isRunning"`
	IsSigned          bool   `json:"isSigned"`
	BinaryHash        string `json:"binaryHash"`
	SignerFingerprint string `json:"signerFingerprint"`
}

func (pr *PostureResponseProcess) Apply(postureData *PostureData) {
	found := false

	pr.SignerFingerprint = CleanHexString(pr.SignerFingerprint)
	pr.BinaryHash = CleanHexString(pr.BinaryHash)

	for i, process := range postureData.Processes {
		if process.PostureCheckId == pr.PostureCheckId {
			postureData.Processes[i] = pr
			postureData.Processes[i].LastUpdatedAt = time.Now()
			found = true
			break
		}
	}

	if !found {
		pr.LastUpdatedAt = time.Now()
		postureData.Processes = append(postureData.Processes, pr)
	}
}

type PostureResponseDomain struct {
	*PostureResponse
	Name string `json:"name"`
}

func (pr *PostureResponseDomain) Apply(postureData *PostureData) {
	postureData.Domain = pr
	postureData.Domain.LastUpdatedAt = time.Now()
}
