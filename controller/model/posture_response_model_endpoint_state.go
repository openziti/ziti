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
	"github.com/michaelquigley/pfxlog"
	"time"
)

type PostureResponseEndpointState struct {
	*PostureResponse
	ApiSessionId string
	WokenAt      *time.Time
	UnlockedAt   *time.Time
}

func (pr *PostureResponseEndpointState) Apply(postureData *PostureData) {

	if postureData.ApiSessions == nil {
		postureData.ApiSessions = map[string]*ApiSessionPostureData{}
	}

	if pr.ApiSessionId == "" {
		pfxlog.Logger().Error("invalid attempt to apply endpoint state posture, empty API Session id")
		return
	}

	if postureData.ApiSessions[pr.ApiSessionId] == nil {
		postureData.ApiSessions[pr.ApiSessionId] = &ApiSessionPostureData{}
	}

	now := time.Now().UTC()

	if postureData.ApiSessions[pr.ApiSessionId].EndpointState == nil {
		state := &PostureResponseEndpointState{}
		state.PostureResponse = &PostureResponse{
			PostureCheckId: pr.PostureCheckId,
			TypeId:         pr.TypeId,
			TimedOut:       false,
			LastUpdatedAt:  time.Now().UTC(),
			SubType:        nil,
		}
		state.PostureResponse.SubType = state

		postureData.ApiSessions[pr.ApiSessionId].EndpointState = state
	}

	if pr.UnlockedAt != nil {
		postureData.ApiSessions[pr.ApiSessionId].EndpointState.UnlockedAt = pr.UnlockedAt
	}

	if pr.WokenAt != nil {
		postureData.ApiSessions[pr.ApiSessionId].EndpointState.WokenAt = pr.WokenAt
	}

	postureData.ApiSessions[pr.ApiSessionId].EndpointState.LastUpdatedAt = now
}
