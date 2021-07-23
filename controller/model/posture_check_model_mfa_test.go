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
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

const (
	mfaTestPostureCheckId = "abc123"
	mfaTestApiSessionId   = "def456"
)

func TestPostureCheckModelMfa(t *testing.T) {
	t.Run("Evaluate", func(t *testing.T) {
		t.Run("returns true for valid MFA check", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns false if API Session data is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			postureData.ApiSessions[mfaTestApiSessionId] = nil

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns false if state is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			postureData.ApiSessions[mfaTestApiSessionId].Mfa = nil

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns true if endpoint state is nil (no wake or unlock events)", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			postureData.ApiSessions[mfaTestApiSessionId].EndpointState = nil

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns false if MFA passedAt is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			postureData.ApiSessions[mfaTestApiSessionId].Mfa.PassedMfaAt = nil

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns false if MFA timed out", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			postureData.ApiSessions[mfaTestApiSessionId].Mfa.TimedOut = true

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns false if MFA timed out with legacy client and not ignoring legacy clients", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			postureData.ApiSessions[mfaTestApiSessionId].Mfa.TimedOut = true
			postureData.ApiSessions[mfaTestApiSessionId].SdkInfo.Version = ""

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns true if MFA timed out with legacy client and ignoring legacy clients", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			mfaCheck.IgnoreLegacyEndpoints = true
			postureData.ApiSessions[mfaTestApiSessionId].Mfa.TimedOut = true
			postureData.ApiSessions[mfaTestApiSessionId].SdkInfo.Version = ""

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns true if woke reported after MFA and inside grace period", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			wokenAt := time.Now().UTC()
			postureData.ApiSessions[mfaTestApiSessionId].EndpointState.WokenAt = &wokenAt

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns true if woke reported after MFA and just inside grace period", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			mfaCheck.TimeoutSeconds = PostureCheckNoTimeout
			wokenAt := time.Now().Add(MfaPromptGracePeriod).Add(1 * time.Second).UTC() //4min 59 seconds ago
			passedMfaAt := wokenAt.Add(-1 * time.Minute)

			postureData.ApiSessions[mfaTestApiSessionId].Mfa.PassedMfaAt = &passedMfaAt
			postureData.ApiSessions[mfaTestApiSessionId].EndpointState.WokenAt = &wokenAt

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns false if woke reported after MFA and outside grace period", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			//set passed mfa before woken, put woken 6 minutes in the past making now() outside of the 5min grace period
			wokenAt := time.Now().Add(MfaPromptGracePeriod).UTC()
			passedMfaAt := wokenAt.Add(-1 * time.Minute)

			postureData.ApiSessions[mfaTestApiSessionId].Mfa.PassedMfaAt = &passedMfaAt
			postureData.ApiSessions[mfaTestApiSessionId].EndpointState.WokenAt = &wokenAt

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns true if unlocked reported after MFA and inside grace period", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			unlockedAt := time.Now().UTC()
			postureData.ApiSessions[mfaTestApiSessionId].EndpointState.UnlockedAt = &unlockedAt

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns true if woke reported after MFA and just inside grace period", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			mfaCheck.TimeoutSeconds = PostureCheckNoTimeout
			wokenAt := time.Now().Add(MfaPromptGracePeriod).Add(1 * time.Second).UTC() //4min 59 seconds ago
			passedMfaAt := wokenAt.Add(-1 * time.Minute)

			postureData.ApiSessions[mfaTestApiSessionId].Mfa.PassedMfaAt = &passedMfaAt
			postureData.ApiSessions[mfaTestApiSessionId].EndpointState.WokenAt = &wokenAt

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns false if unlocked reported after MFA and outside grace period", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			//set passed mfa before woken, put woken 6 minutes in the past making now() outside of the 5min grace period
			unlockedAt := time.Now().Add(MfaPromptGracePeriod).UTC()
			passedMfaAt := unlockedAt.Add(-1 * time.Minute)

			postureData.ApiSessions[mfaTestApiSessionId].Mfa.PassedMfaAt = &passedMfaAt
			postureData.ApiSessions[mfaTestApiSessionId].EndpointState.UnlockedAt = &unlockedAt

			result := mfaCheck.Evaluate(mfaTestApiSessionId, postureData)

			req := require.New(t)
			req.False(result)
		})
	})

	t.Run("PassedOnWake", func(t *testing.T) {

		t.Run("returns true with default case", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			result := mfaCheck.PassedOnWake(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns true when PromptOnWake is false", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			mfaCheck.PromptOnWake = false

			result := mfaCheck.PassedOnWake(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns false when PromptOnWake is true and ApiSessionData is nil", func(t *testing.T) {
			mfaCheck, _ := newMfaCheckAndPostureData()

			result := mfaCheck.PassedOnWake(nil)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns false when PromptOnWake is true and MFA state is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]

			apiSessionData.Mfa = nil

			result := mfaCheck.PassedOnWake(apiSessionData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns false when PromptOnWake is true and MFA PassedAt is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]

			apiSessionData.Mfa.PassedMfaAt = nil

			result := mfaCheck.PassedOnWake(apiSessionData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns true when PromptOnWake is true and endpoint state is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]

			apiSessionData.EndpointState = nil

			result := mfaCheck.PassedOnWake(apiSessionData)

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns true when PromptOnWake is true and endpoint state WokenAt is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]

			apiSessionData.EndpointState.WokenAt = nil

			result := mfaCheck.PassedOnWake(apiSessionData)

			req := require.New(t)
			req.True(result)
		})
	})

	t.Run("PassedOnUnlock", func(t *testing.T) {

		t.Run("returns true with default case", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			result := mfaCheck.PassedOnUnlock(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns true when PromptOnUnlock is false", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()

			mfaCheck.PromptOnUnlock = false

			result := mfaCheck.PassedOnUnlock(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns false when PromptOnUnlock is true and ApiSessionData is nil", func(t *testing.T) {
			mfaCheck, _ := newMfaCheckAndPostureData()

			result := mfaCheck.PassedOnUnlock(nil)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns false when PromptOnUnlock is true and MFA state is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]

			apiSessionData.Mfa = nil

			result := mfaCheck.PassedOnUnlock(apiSessionData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns false when PromptOnUnlock is true and MFA PassedAt is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]

			apiSessionData.Mfa.PassedMfaAt = nil

			result := mfaCheck.PassedOnUnlock(apiSessionData)

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns true when PromptOnUnlock is true and endpoint state is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]

			apiSessionData.EndpointState = nil

			result := mfaCheck.PassedOnUnlock(apiSessionData)

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns true when PromptOnUnlock is true and endpoint state UnlockedAt is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]

			apiSessionData.EndpointState.UnlockedAt = nil

			result := mfaCheck.PassedOnUnlock(apiSessionData)

			req := require.New(t)
			req.True(result)
		})
	})

	t.Run("FailureValues", func(t *testing.T) {
		t.Run("expected values for default case are correct", func(t *testing.T) {
			req := require.New(t)
			mfaCheck, postureData := newMfaCheckAndPostureData()
			val := mfaCheck.FailureValues(mfaTestApiSessionId, postureData)
			failureValues := val.(*PostureCheckFailureValuesMfa)

			req.NotEmpty(failureValues)
			req.True(failureValues.ExpectedValue.PassedMfa)
			req.True(failureValues.ExpectedValue.PassedOnWake)
			req.True(failureValues.ExpectedValue.PassedOnUnlock)
			req.False(failureValues.ExpectedValue.TimedOutSeconds)
		})

		t.Run("actual values for default case are correct", func(t *testing.T) {
			req := require.New(t)
			mfaCheck, postureData := newMfaCheckAndPostureData()
			val := mfaCheck.FailureValues(mfaTestApiSessionId, postureData)
			failureValues := val.(*PostureCheckFailureValuesMfa)

			req.NotEmpty(failureValues)
			req.True(failureValues.ActualValue.PassedMfa)
			req.True(failureValues.ActualValue.PassedOnWake)
			req.True(failureValues.ActualValue.PassedOnUnlock)
			req.False(failureValues.ActualValue.TimedOutSeconds)
		})

		t.Run("actual values for for invalid API Session are correct", func(t *testing.T) {
			req := require.New(t)
			mfaCheck, postureData := newMfaCheckAndPostureData()
			val := mfaCheck.FailureValues("invalid_api_session_id", postureData)
			failureValues := val.(*PostureCheckFailureValuesMfa)

			req.NotEmpty(failureValues)
			req.False(failureValues.ActualValue.PassedMfa)
			req.False(failureValues.ActualValue.PassedOnWake)
			req.False(failureValues.ActualValue.PassedOnUnlock)
			req.True(failureValues.ActualValue.TimedOutSeconds)
		})
	})

	t.Run("IsLegacyClient", func(t *testing.T) {
		t.Run("returns false for ziti-sdk-c and  0.25.0", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			result := mfaCheck.IsLegacyClient(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns true for ziti-sdk-c and  0.24.4", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]
			apiSessionData.SdkInfo.Version = "0.24.4"
			result := mfaCheck.IsLegacyClient(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns true for ziti-sdk-c and  0.24.5", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]
			apiSessionData.SdkInfo.Version = "0.24.5"
			result := mfaCheck.IsLegacyClient(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns true for missing type and  0.24.4", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]
			apiSessionData.SdkInfo.Version = "0.24.4"
			apiSessionData.SdkInfo.Type = ""
			result := mfaCheck.IsLegacyClient(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns false for ziti-sdk-c and  1.0.0", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]
			apiSessionData.SdkInfo.Version = "1.0.0"
			result := mfaCheck.IsLegacyClient(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.False(result)
		})

		t.Run("returns true if sdk info is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]
			apiSessionData.SdkInfo = nil
			result := mfaCheck.IsLegacyClient(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.True(result)
		})

		t.Run("returns true if for ziti-sdk-c and an invalid version", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			apiSessionData := postureData.ApiSessions[mfaTestApiSessionId]
			apiSessionData.SdkInfo.Version = "weeeeeee"
			result := mfaCheck.IsLegacyClient(postureData.ApiSessions[mfaTestApiSessionId])

			req := require.New(t)
			req.True(result)
		})
	})

	t.Run("GetTimeoutSeconds", func(t *testing.T) {
		t.Run("returns the static timeout value", func(t *testing.T) {
			mfaCheck, _ := newMfaCheckAndPostureData()

			req := require.New(t)
			req.Equal(mfaCheck.TimeoutSeconds, mfaCheck.GetTimeoutSeconds())
		})

		t.Run("returns no timeout for negative numbers", func(t *testing.T) {
			mfaCheck, _ := newMfaCheckAndPostureData()
			mfaCheck.TimeoutSeconds = -100
			req := require.New(t)
			req.Equal(PostureCheckNoTimeout, mfaCheck.GetTimeoutSeconds())
		})

		t.Run("returns no timeout for 0", func(t *testing.T) {
			mfaCheck, _ := newMfaCheckAndPostureData()
			mfaCheck.TimeoutSeconds = 0
			req := require.New(t)
			req.Equal(PostureCheckNoTimeout, mfaCheck.GetTimeoutSeconds())
		})
	})

	t.Run("GetTimeoutRemainingSeconds", func(t *testing.T) {
		t.Run("returns 0 if no ApiSessionData is present", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			postureData.ApiSessions = map[string]*ApiSessionPostureData{}
			req := require.New(t)
			req.Equal(int64(0), mfaCheck.GetTimeoutRemainingSeconds(mfaTestApiSessionId, postureData))
		})

		t.Run("returns 0 if no MFA data is present", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			postureData.ApiSessions[mfaTestApiSessionId].Mfa = nil
			req := require.New(t)
			req.Equal(int64(0), mfaCheck.GetTimeoutRemainingSeconds(mfaTestApiSessionId, postureData))
		})

		t.Run("returns 0 if no last MFA passedAt is nil", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			postureData.ApiSessions[mfaTestApiSessionId].Mfa.PassedMfaAt = nil
			req := require.New(t)
			req.Equal(int64(0), mfaCheck.GetTimeoutRemainingSeconds(mfaTestApiSessionId, postureData))
		})

		t.Run("returns near timeout if much time hasn't passed", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			req := require.New(t)

			//time based, if on a slow machine this might slide
			req.LessOrEqual(mfaCheck.GetTimeoutRemainingSeconds(mfaTestApiSessionId, postureData), mfaCheck.TimeoutSeconds)
			req.GreaterOrEqual(mfaCheck.GetTimeoutRemainingSeconds(mfaTestApiSessionId, postureData), mfaCheck.TimeoutSeconds-5)
		})

		t.Run("returns not timeout for legacy clients if ignore legacy is true", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			mfaCheck.IgnoreLegacyEndpoints = true
			postureData.ApiSessions[mfaTestApiSessionId].SdkInfo.Version = "0.24.4"

			req := require.New(t)
			req.Equal(PostureCheckNoTimeout, mfaCheck.GetTimeoutRemainingSeconds(mfaTestApiSessionId, postureData))
		})

		t.Run("returns near timeout for legacy client if ignore legacy is false", func(t *testing.T) {
			mfaCheck, postureData := newMfaCheckAndPostureData()
			postureData.ApiSessions[mfaTestApiSessionId].SdkInfo.Version = "0.24.4"
			req := require.New(t)

			//time based, if on a slow machine this might slide
			req.LessOrEqual(mfaCheck.GetTimeoutRemainingSeconds(mfaTestApiSessionId, postureData), mfaCheck.TimeoutSeconds)
			req.GreaterOrEqual(mfaCheck.GetTimeoutRemainingSeconds(mfaTestApiSessionId, postureData), mfaCheck.TimeoutSeconds-5)
		})
	})
	
	t.Run("calculateTimeout", func(t *testing.T) {
		
		t.Run("eventAt == now, results in 5m", func(t *testing.T) {
			req := require.New(t)

			now, err := time.Parse(time.RFC3339, "2020-01-01T11:30:00Z")
			req.NoError(err)

			eventAt, err := time.Parse(time.RFC3339, "2020-01-01T11:30:00Z")
			req.NoError(err)

			result := calculateTimeout(now, eventAt, -1 * MfaPromptGracePeriod)

			req.Equal(int64(300), result)
		})

		t.Run("eventAt 3min ago, results in 2m", func(t *testing.T) {
			req := require.New(t)

			now, err := time.Parse(time.RFC3339, "2020-01-01T11:30:00Z")
			req.NoError(err)

			eventAt, err := time.Parse(time.RFC3339, "2020-01-01T11:27:00Z")
			req.NoError(err)

			result := calculateTimeout(now, eventAt, -1 * MfaPromptGracePeriod)

			req.Equal(int64(120), result)
		})

		t.Run("eventAt 5m before now, results in 0", func(t *testing.T) {
			req := require.New(t)

			now, err := time.Parse(time.RFC3339, "2020-01-01T11:30:00Z")
			req.NoError(err)

			eventAt, err := time.Parse(time.RFC3339, "2020-01-01T11:25:00Z")
			req.NoError(err)

			result := calculateTimeout(now, eventAt, -1 * MfaPromptGracePeriod)

			req.Equal(int64(0), result)
		})

		t.Run("eventAt 10m before now, results in 0", func(t *testing.T) {
			req := require.New(t)

			now, err := time.Parse(time.RFC3339, "2020-01-01T11:30:00Z")
			req.NoError(err)

			eventAt, err := time.Parse(time.RFC3339, "2020-01-01T11:20:00Z")
			req.NoError(err)

			result := calculateTimeout(now, eventAt, -1 * MfaPromptGracePeriod)

			req.Equal(int64(0), result)
		})
	})
}

// newMfaCheckAndPostureData returns a MFA posture check and posture data that will
// pass evaluation. The posture check will evaluate promptOnWake and promptOnUnlock
// with events occurring 10 seconds in the past.
func newMfaCheckAndPostureData() (*PostureCheckMfa, *PostureData) {
	passedMfaAt := time.Now().UTC()
	lastUpdatedAt := passedMfaAt

	wokenAt := time.Now().Add(-10 * time.Second)
	unlockedAt := time.Now().Add(-10 * time.Second)

	postureResponse := &PostureResponse{
		PostureCheckId: mfaTestPostureCheckId,
		TypeId:         PostureCheckTypeMFA,
		TimedOut:       false,
		LastUpdatedAt:  lastUpdatedAt,
	}

	postureResponseMfa := &PostureResponseMfa{
		PostureResponse: nil,
		ApiSessionId:    mfaTestApiSessionId,
		PassedMfaAt:     &passedMfaAt,
	}

	postureResponse.SubType = postureResponseMfa
	postureResponseMfa.PostureResponse = postureResponse

	validPostureData := &PostureData{
		ApiSessions: map[string]*ApiSessionPostureData{
			mfaTestApiSessionId: {
				Mfa: postureResponseMfa,
				EndpointState: &PostureResponseEndpointState{
					ApiSessionId: mfaTestApiSessionId,
					WokenAt:      &wokenAt,
					UnlockedAt:   &unlockedAt,
				},
				SdkInfo: &SdkInfo{
					Type:    "ziti-sdk-c",
					Version: "0.25.0",
				},
			},
		},
	}

	mfaCheck := &PostureCheckMfa{
		TimeoutSeconds: 60,
		PromptOnWake:   true,
		PromptOnUnlock: true,
	}

	return mfaCheck, validPostureData
}
