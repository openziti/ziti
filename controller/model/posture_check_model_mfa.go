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

func (p *PostureCheckMfa) IsLegacyClient(apiSessionData *ApiSessionPostureData) bool {
	if apiSessionData.SdkInfo != nil && apiSessionData.SdkInfo.Type == ZitiSdkTypeC {
		if ver, err := semver.Parse(apiSessionData.SdkInfo.Version); err == nil {
			return ver.LT(minCSdkVersion)
		}
	}

	return false
}

func (p *PostureCheckMfa) GetTimeoutSeconds(apiSessionId string, pd *PostureData) int64 {
	if p.TimeoutSeconds == PostureCheckNoTimeout {
		return PostureCheckNoTimeout
	}

	apiSessionData := pd.ApiSessions[apiSessionId]

	if apiSessionData == nil || apiSessionData.Mfa == nil || apiSessionData.Mfa.PassedMfaAt == nil {
		return 0
	}

	// for legacy endpoints return no timeout
	if p.IgnoreLegacyEndpoints && p.IsLegacyClient(apiSessionData) {
		return PostureCheckNoTimeout
	}

	timeSinceLastMfa := time.Now().Sub(*apiSessionData.Mfa.PassedMfaAt)

	timeout := p.TimeoutSeconds - int64(timeSinceLastMfa.Seconds())
	if timeout < 0 {
		return 0
	}

	if p.PromptOnWake && apiSessionData.EndpointState != nil && apiSessionData.EndpointState.WokenAt != nil {
		onWakeTimeOut := int64(time.Now().Sub(apiSessionData.EndpointState.WokenAt.Add(MfaPromptGracePeriod)).Seconds())

		if onWakeTimeOut < 0 {
			onWakeTimeOut = 0
		}

		if onWakeTimeOut < timeout {
			timeout = onWakeTimeOut
		}
	}

	if p.PromptOnUnlock && apiSessionData.EndpointState != nil && apiSessionData.EndpointState.UnlockedAt != nil {
		onUnlockTimeOut := int64(time.Now().Sub(apiSessionData.EndpointState.UnlockedAt.Add(MfaPromptGracePeriod)).Seconds())

		if onUnlockTimeOut < 0 {
			onUnlockTimeOut = 0
		}

		if onUnlockTimeOut < timeout {
			timeout = onUnlockTimeOut
		}
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
	}

	if apiSessionData, ok := pd.ApiSessions[apiSessionId]; ok {
		if apiSessionData.Mfa != nil {
			ret.ActualValue.TimedOutSeconds = apiSessionData.Mfa.TimedOut
			ret.ActualValue.PassedMfa = apiSessionData.Mfa.PassedMfaAt != nil
		}
		ret.ActualValue.PassedOnUnlock = p.PassedOnUnlock(apiSessionData)
		ret.ActualValue.PassedOnWake = p.PassedOnWake(apiSessionData)
	}

	return ret
}

func (p *PostureCheckMfa) Evaluate(apiSessionId string, pd *PostureData) bool {
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
	now := time.Now().UTC()
	timeoutSeconds := p.GetTimeoutSeconds(apiSessionId, pd)

	if timeoutSeconds != PostureCheckNoTimeout {
		expiresAt := apiSessionData.Mfa.PassedMfaAt.Add(time.Duration(timeoutSeconds) * time.Second)
		if expiresAt.Before(now) {
			apiSessionData.Mfa.TimedOut = true
		}
	}

	if apiSessionData.Mfa.TimedOut {
		return false
	}

	if !p.PassedOnWake(apiSessionData) {
		return false
	}

	if !p.PassedOnUnlock(apiSessionData) {
		return false
	}

	return true
}

func (p *PostureCheckMfa) PassedOnWake(apiSessionData *ApiSessionPostureData) bool {
	if !p.PromptOnWake {
		return true
	}

	if apiSessionData == nil {
		return false
	}

	if apiSessionData.Mfa == nil || apiSessionData.Mfa.PassedMfaAt == nil {
		return false
	}
	now := time.Now().UTC()
	if apiSessionData.EndpointState == nil || apiSessionData.EndpointState.WokenAt == nil || apiSessionData.EndpointState.WokenAt.Before(*apiSessionData.Mfa.PassedMfaAt) || now.Add(MfaPromptGracePeriod).Before(*apiSessionData.EndpointState.WokenAt) {
		return true
	}

	return false
}

func (p *PostureCheckMfa) PassedOnUnlock(apiSessionData *ApiSessionPostureData) bool {
	if !p.PromptOnUnlock {
		return true
	}

	if apiSessionData == nil {
		return false
	}

	if apiSessionData.Mfa == nil || apiSessionData.Mfa.PassedMfaAt == nil {
		return false
	}

	now := time.Now().UTC()
	if apiSessionData.EndpointState == nil || apiSessionData.EndpointState.UnlockedAt == nil || apiSessionData.EndpointState.UnlockedAt.Before(*apiSessionData.Mfa.PassedMfaAt) || now.Add(MfaPromptGracePeriod).Before(*apiSessionData.EndpointState.UnlockedAt) {
		return true
	}

	return false
}

func newPostureCheckMfa() PostureCheckSubType {
	return &PostureCheckMfa{}
}

func (p *PostureCheckMfa) fillFrom(handler Handler, tx *bbolt.Tx, check *persistence.PostureCheck, subType persistence.PostureCheckSubType) error {
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

func (p *PostureCheckMfa) toBoltEntityForCreate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return &persistence.PostureCheckMfa{
		TimeoutSeconds:        p.TimeoutSeconds,
		PromptOnWake:          p.PromptOnWake,
		PromptOnUnlock:        p.PromptOnUnlock,
		IgnoreLegacyEndpoints: p.IgnoreLegacyEndpoints,
	}, nil
}

func (p *PostureCheckMfa) toBoltEntityForUpdate(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return p.toBoltEntityForCreate(tx, handler)
}

func (p *PostureCheckMfa) toBoltEntityForPatch(tx *bbolt.Tx, handler Handler) (persistence.PostureCheckSubType, error) {
	return p.toBoltEntityForCreate(tx, handler)
}

type PostureCheckMfaValues struct {
	TimedOutSeconds       bool
	PassedMfa             bool
	PassedOnWake          bool
	PassedOnUnlock        bool
	IgnoreLegacyEndpoints bool
}

type PostureCheckFailureValuesMfa struct {
	ActualValue   PostureCheckMfaValues
	ExpectedValue PostureCheckMfaValues
}

func (p PostureCheckFailureValuesMfa) Expected() interface{} {
	return p.ExpectedValue
}

func (p PostureCheckFailureValuesMfa) Actual() interface{} {
	return p.ActualValue
}
