/*
	Copyright NetFoundry Inc.

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
	"testing"
	"time"

	"github.com/kataras/go-events"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
)

// newSeedTestManager returns a PostureResponseManager over a bare posture cache: no env, no background tasks.
func newSeedTestManager() (*PostureResponseManager, *int) {
	pc := &PostureCache{
		identityToPostureData:    cmap.New[*PostureData](),
		apiSessionIdToIdentityId: cmap.New[string](),
		EventEmmiter:             events.New(),
	}
	emitted := 0
	pc.On(EventIdentityPostureDataAltered, func(...interface{}) { emitted++ })
	return &PostureResponseManager{postureCache: pc}, &emitted
}

// Test_SeedMfaPosture_PassesMfaCheck locks in GH-4558: an OIDC api session whose token says amr=totp passes MFA
// posture checks at auth_time, the same as a legacy /authenticate/mfa success.
func Test_SeedMfaPosture_PassesMfaCheck(t *testing.T) {
	manager, emitted := newSeedTestManager()
	authTime := time.Now().Add(-time.Minute).UTC()

	require.True(t, manager.SeedMfaPosture("id1", "as1", authTime))
	require.Equal(t, 1, *emitted, "seeding alerts posture listeners, so clients are told of the service change")

	pd := manager.postureCache.PostureData("id1")
	require.NotNil(t, pd.ApiSessions["as1"].GetPassedMfaAt())
	require.True(t, authTime.Equal(*pd.ApiSessions["as1"].GetPassedMfaAt()), "baseline comes from auth_time")

	check := &PostureCheckMfa{TimeoutSeconds: PostureCheckNoTimeout}
	require.True(t, check.Evaluate("as1", pd))
	require.False(t, check.Evaluate("as2", pd), "other api sessions are not seeded")

	// a timeout counts from auth_time
	require.True(t, (&PostureCheckMfa{TimeoutSeconds: 3600}).Evaluate("as1", pd))
	require.False(t, (&PostureCheckMfa{TimeoutSeconds: 30}).Evaluate("as1", pd))
}

// Test_SeedMfaPosture_NeverOverwrites locks in that every request of the session seeds again, but only the first
// does anything: a later MFA code is not moved back to auth_time, and MFA removed since stays removed.
func Test_SeedMfaPosture_NeverOverwrites(t *testing.T) {
	t.Run("seeded session", func(t *testing.T) {
		manager, emitted := newSeedTestManager()
		first := time.Now().Add(-time.Hour).UTC()

		require.True(t, manager.SeedMfaPosture("id1", "as1", first))
		require.False(t, manager.SeedMfaPosture("id1", "as1", time.Now().UTC()))
		require.Equal(t, 1, *emitted)
		require.True(t, first.Equal(*manager.postureCache.PostureData("id1").ApiSessions["as1"].GetPassedMfaAt()))
	})

	t.Run("later MFA code", func(t *testing.T) {
		manager, _ := newSeedTestManager()
		manager.SetMfaPosture("id1", "as1", true)
		passedAt := *manager.postureCache.PostureData("id1").ApiSessions["as1"].GetPassedMfaAt()

		require.False(t, manager.SeedMfaPosture("id1", "as1", time.Now().Add(-time.Hour).UTC()))
		require.True(t, passedAt.Equal(*manager.postureCache.PostureData("id1").ApiSessions["as1"].GetPassedMfaAt()))
	})

	t.Run("MFA removed", func(t *testing.T) {
		manager, _ := newSeedTestManager()
		require.True(t, manager.SeedMfaPosture("id1", "as1", time.Now().Add(-time.Minute).UTC()))
		manager.SetMfaPostureForIdentity("id1", false)

		require.False(t, manager.SeedMfaPosture("id1", "as1", time.Now().Add(-time.Minute).UTC()))
		pd := manager.postureCache.PostureData("id1")
		require.Nil(t, pd.ApiSessions["as1"].GetPassedMfaAt())
		require.False(t, (&PostureCheckMfa{TimeoutSeconds: PostureCheckNoTimeout}).Evaluate("as1", pd))
	})
}
