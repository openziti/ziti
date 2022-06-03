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
	"github.com/blang/semver"
	"github.com/openziti/edge/controller/persistence"
	"go.etcd.io/bbolt"
	"time"
)

var _ PostureCheckSubType = &PostureCheckMfa{}
var minCSdkVersion semver.Version

const ZitiSdkTypeC = "ziti-sdk-c"
const MfaPromptGracePeriod = -5 * time.Minute //5m

func init() {
	minCSdkVersion = semver.MustParse("0.25.0")
}

type PostureCheckMfa struct {
	TimeoutSeconds        int64
	PromptOnWake          bool
	PromptOnUnlock        bool
	IgnoreLegacyEndpoints bool
}

func (p *PostureCheckMfa) LastUpdatedAt(apiSessionId string, pd *PostureData) *time.Time {
	apiSessionData := pd.ApiSessions[apiSessionId]

	//not enough data yet
	if apiSessionData == nil || apiSessionData.Mfa == nil || apiSessionData.EndpointState == nil {
		return nil
	}

	var ret *time.Time = nil

	if p.PromptOnWake && apiSessionData.EndpointState.WokenAt != nil {
		ret = apiSessionData.EndpointState.WokenAt
	}

	if p.PromptOnUnlock && apiSessionData.EndpointState.UnlockedAt != nil {
		if ret == nil || apiSessionData.EndpointState.UnlockedAt.After(*ret) {
			ret = apiSessionData.EndpointState.UnlockedAt
		}
	}

	return ret
}

func (p *PostureCheckMfa) IsLegacyClient(apiSessionData *ApiSessionPostureData) bool {
	if apiSessionData.SdkInfo == nil {
		return true // don't know what it is
	}

	//c-sdk, return true/false based on version
	if apiSessionData.SdkInfo.Type == ZitiSdkTypeC {
		if ver, err := semver.Parse(apiSessionData.SdkInfo.Version); err == nil {
			return ver.LT(minCSdkVersion)
		}
	}

	//else not sure what it is return true
	return true
}

func (p *PostureCheckMfa) GetTimeoutSeconds() int64 {
	if p.TimeoutSeconds > 0 {
		return p.TimeoutSeconds
	}

	return PostureCheckNoTimeout
}

func (p *PostureCheckMfa) GetTimeoutRemainingSeconds(apiSessionId string, pd *PostureData) int64 {
	return p.getTimeoutRemainingAtSeconds(apiSessionId, pd, time.Now())
}

func (p *PostureCheckMfa) getTimeoutRemainingAtSeconds(apiSessionId string, pd *PostureData, now time.Time) int64 {
	if p.TimeoutSeconds == PostureCheckNoTimeout {
		return PostureCheckNoTimeout
	}

	apiSessionData := pd.ApiSessions[apiSessionId]

	// no MFA data, return 0 as we have a timeout of some value
	if apiSessionData == nil || apiSessionData.Mfa == nil || apiSessionData.Mfa.PassedMfaAt == nil {
		return 0
	}

	// for legacy endpoints return no timeout
	if p.IgnoreLegacyEndpoints && p.IsLegacyClient(apiSessionData) {
		return PostureCheckNoTimeout
	}

	timeSinceLastMfa := now.Sub(*apiSessionData.Mfa.PassedMfaAt)

	timeoutRemaining := p.TimeoutSeconds - int64(timeSinceLastMfa.Seconds())
	if timeoutRemaining <= 0 {
		return 0
	}

	if p.PromptOnWake && apiSessionData.EndpointState != nil && apiSessionData.EndpointState.WokenAt != nil {
		if apiSessionData.Mfa.PassedMfaAt == nil || apiSessionData.EndpointState.WokenAt.After(*apiSessionData.Mfa.PassedMfaAt) {
			onWakeTimeOut := calculateTimeout(now, *apiSessionData.EndpointState.WokenAt, -1*MfaPromptGracePeriod)

			if onWakeTimeOut < timeoutRemaining {
				timeoutRemaining = onWakeTimeOut
			}
		}
	}

	if p.PromptOnUnlock && apiSessionData.EndpointState != nil && apiSessionData.EndpointState.UnlockedAt != nil {
		if apiSessionData.Mfa.PassedMfaAt == nil || apiSessionData.EndpointState.UnlockedAt.After(*apiSessionData.Mfa.PassedMfaAt) {
			onUnlockTimeout := calculateTimeout(now, *apiSessionData.EndpointState.UnlockedAt, -1*MfaPromptGracePeriod)

			if onUnlockTimeout < timeoutRemaining {
				timeoutRemaining = onUnlockTimeout
			}
		}
	}

	return timeoutRemaining
}

func calculateTimeout(now time.Time, pastEventOccuredAt time.Time, gracePeriod time.Duration) int64 {
	durationSinceEvent := now.Sub(pastEventOccuredAt)

	timeout := int64((gracePeriod - durationSinceEvent).Seconds())

	if timeout < 0 {
		timeout = 0
	}

	return timeout
}

func (p *PostureCheckMfa) FailureValues(apiSessionId string, pd *PostureData) PostureCheckFailureValues {
	ret := &PostureCheckFailureValuesMfa{
		ActualValue: PostureCheckMfaValues{
			PassedMfa:             false,
			TimedOutSeconds:       true,
			PassedOnWake:          false,
			PassedOnUnlock:        false,
			IgnoreLegacyEndpoints: false,
		},
		ExpectedValue: PostureCheckMfaValues{
			PassedMfa:             true,
			PassedOnWake:          p.PromptOnWake,
			PassedOnUnlock:        p.PromptOnUnlock,
			TimedOutSeconds:       false,
			IgnoreLegacyEndpoints: p.IgnoreLegacyEndpoints,
		},
		Criteria: PostureCheckMfaCriteria{
			PassedMfaAt:             nil,
			WokenAt:                 nil,
			UnlockedAt:              nil,
			TimeoutSeconds:          0,
			TimeoutRemainingSeconds: 0,
		},
	}

	ret.Criteria.TimeoutSeconds = p.GetTimeoutSeconds()

	if apiSessionData, ok := pd.ApiSessions[apiSessionId]; ok {
		if apiSessionData.Mfa != nil {
			ret.ActualValue.TimedOutSeconds = apiSessionData.Mfa.TimedOut
			ret.ActualValue.PassedMfa = apiSessionData.Mfa.PassedMfaAt != nil

			ret.Criteria.PassedMfaAt = apiSessionData.Mfa.PassedMfaAt
			ret.Criteria.TimeoutRemainingSeconds = p.GetTimeoutRemainingSeconds(apiSessionId, pd)
		}

		if apiSessionData.EndpointState != nil {
			ret.Criteria.UnlockedAt = apiSessionData.EndpointState.UnlockedAt
			ret.Criteria.WokenAt = apiSessionData.EndpointState.WokenAt
		}

		now := time.Now().UTC()
		ret.ActualValue.PassedOnUnlock = p.PassedOnUnlock(apiSessionData, now)
		ret.ActualValue.PassedOnWake = p.PassedOnWake(apiSessionData, now)
	}

	return ret
}

func (p *PostureCheckMfa) Evaluate(apiSessionId string, pd *PostureData) bool {
	return p.evaluateAt(apiSessionId, pd, time.Now())
}

func (p *PostureCheckMfa) evaluateAt(apiSessionId string, pd *PostureData, now time.Time) bool {
	apiSessionData := pd.ApiSessions[apiSessionId]

	if apiSessionData == nil {
		return false
	}

	if apiSessionData.Mfa == nil {
		return false
	}

	if apiSessionData.Mfa.PassedMfaAt == nil {
		return false
	}

	if p.TimeoutSeconds != PostureCheckNoTimeout {
		expiresAt := apiSessionData.Mfa.PassedMfaAt.Add(time.Duration(p.TimeoutSeconds) * time.Second)
		if expiresAt.Before(now) {
			apiSessionData.Mfa.TimedOut = true
		}
	}

	if apiSessionData.Mfa.TimedOut {
		return false
	}

	if !p.PassedOnWake(apiSessionData, now) {
		return false
	}

	if !p.PassedOnUnlock(apiSessionData, now) {
		return false
	}

	return true
}

func (p *PostureCheckMfa) PassedOnWake(apiSessionData *ApiSessionPostureData, now time.Time) bool {
	if !p.PromptOnWake {
		return true
	}

	if apiSessionData == nil {
		return false
	}

	if apiSessionData.Mfa == nil || apiSessionData.Mfa.PassedMfaAt == nil {
		return false
	}

	if apiSessionData.EndpointState == nil || apiSessionData.EndpointState.WokenAt == nil || apiSessionData.EndpointState.WokenAt.Before(*apiSessionData.Mfa.PassedMfaAt) || now.Add(MfaPromptGracePeriod).Before(*apiSessionData.EndpointState.WokenAt) {
		return true
	}

	return false
}

func (p *PostureCheckMfa) PassedOnUnlock(apiSessionData *ApiSessionPostureData, now time.Time) bool {
	if !p.PromptOnUnlock {
		return true
	}

	if apiSessionData == nil {
		return false
	}

	if apiSessionData.Mfa == nil || apiSessionData.Mfa.PassedMfaAt == nil {
		return false
	}

	if apiSessionData.EndpointState == nil || apiSessionData.EndpointState.UnlockedAt == nil || apiSessionData.EndpointState.UnlockedAt.Before(*apiSessionData.Mfa.PassedMfaAt) || now.Add(MfaPromptGracePeriod).Before(*apiSessionData.EndpointState.UnlockedAt) {
		return true
	}

	return false
}

func newPostureCheckMfa() PostureCheckSubType {
	return &PostureCheckMfa{}
}

func (p *PostureCheckMfa) fillFrom(handler EntityManager, tx *bbolt.Tx, check *persistence.PostureCheck, subType persistence.PostureCheckSubType) error {
	subCheck := subType.(*persistence.PostureCheckMfa)

	if subCheck == nil {
		return fmt.Errorf("could not covert mfa check to bolt type")
	}

	p.TimeoutSeconds = subCheck.TimeoutSeconds
	p.PromptOnWake = subCheck.PromptOnWake
	p.PromptOnUnlock = subCheck.PromptOnUnlock
	p.IgnoreLegacyEndpoints = subCheck.IgnoreLegacyEndpoints

	return nil
}

func (p *PostureCheckMfa) toBoltEntityForCreate(tx *bbolt.Tx, handler EntityManager) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckMfa{
		TimeoutSeconds:        p.TimeoutSeconds,
		PromptOnWake:          p.PromptOnWake,
		PromptOnUnlock:        p.PromptOnUnlock,
		IgnoreLegacyEndpoints: p.IgnoreLegacyEndpoints,
	}, nil
}

func (p *PostureCheckMfa) toBoltEntityForUpdate(tx *bbolt.Tx, handler EntityManager) (persistence.PostureCheckSubType, error) {
	return p.toBoltEntityForCreate(tx, handler)
}

func (p *PostureCheckMfa) toBoltEntityForPatch(tx *bbolt.Tx, handler EntityManager) (persistence.PostureCheckSubType, error) {
	return p.toBoltEntityForCreate(tx, handler)
}

type PostureCheckMfaValues struct {
	TimedOutSeconds       bool
	PassedMfa             bool
	PassedOnWake          bool
	PassedOnUnlock        bool
	IgnoreLegacyEndpoints bool
}

type PostureCheckMfaCriteria struct {
	PassedMfaAt             *time.Time
	WokenAt                 *time.Time
	UnlockedAt              *time.Time
	TimeoutSeconds          int64
	TimeoutRemainingSeconds int64
}

type PostureCheckFailureValuesMfa struct {
	ActualValue   PostureCheckMfaValues
	ExpectedValue PostureCheckMfaValues
	Criteria      PostureCheckMfaCriteria
}

func (p PostureCheckFailureValuesMfa) Expected() interface{} {
	return p.ExpectedValue
}

func (p PostureCheckFailureValuesMfa) Actual() interface{} {
	return p.ActualValue
}
