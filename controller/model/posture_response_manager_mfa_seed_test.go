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
	"errors"
	"testing"
	"time"

	"github.com/kataras/go-events"
	"github.com/openziti/ziti/v2/controller/db"
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
	manager := &PostureResponseManager{
		postureCache: pc,
		// enrolled long before any auth_time the tests use
		mfaEnrolledAt: func(string) (time.Time, bool, error) {
			return time.Now().Add(-24 * time.Hour), true, nil
		},
	}
	return manager, &emitted
}

// Test_SeedMfaPosture_RequiresCurrentEnrollment locks in that amr=totp is only trusted while the identity still has
// the verified MFA enrollment the token was issued under. Refresh keeps amr, so a token can outlive its MFA.
func Test_SeedMfaPosture_RequiresCurrentEnrollment(t *testing.T) {
	authTime := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	mfaCheck := &PostureCheckMfa{TimeoutSeconds: PostureCheckNoTimeout}

	t.Run("no verified MFA", func(t *testing.T) {
		manager, emitted := newSeedTestManager()
		lookups := 0
		manager.mfaEnrolledAt = func(string) (time.Time, bool, error) {
			lookups++
			return time.Time{}, false, nil
		}

		require.False(t, manager.SeedMfaPosture("id1", "as1", authTime))
		require.False(t, manager.SeedMfaPosture("id1", "as1", authTime))
		require.Equal(t, 1, lookups, "the failed result is cached, not looked up on every request")
		require.Equal(t, 0, *emitted)
		require.False(t, mfaCheck.Evaluate("as1", manager.postureCache.PostureData("id1")))
	})

	t.Run("re-enrolled after auth_time", func(t *testing.T) {
		manager, _ := newSeedTestManager()
		manager.mfaEnrolledAt = func(string) (time.Time, bool, error) {
			return authTime.Add(time.Second), true, nil
		}

		require.False(t, manager.SeedMfaPosture("id1", "as1", authTime))
		require.False(t, mfaCheck.Evaluate("as1", manager.postureCache.PostureData("id1")))
	})

	t.Run("enrolled in the same second as auth_time", func(t *testing.T) {
		manager, _ := newSeedTestManager()
		manager.mfaEnrolledAt = func(string) (time.Time, bool, error) {
			return authTime.Add(500 * time.Millisecond), true, nil
		}

		require.True(t, manager.SeedMfaPosture("id1", "as1", authTime))
	})

	t.Run("lookup error", func(t *testing.T) {
		manager, _ := newSeedTestManager()
		manager.mfaEnrolledAt = func(string) (time.Time, bool, error) {
			return time.Time{}, false, errors.New("boom")
		}

		require.False(t, manager.SeedMfaPosture("id1", "as1", authTime))

		manager.mfaEnrolledAt = func(string) (time.Time, bool, error) {
			return authTime.Add(-time.Hour), true, nil
		}
		require.True(t, manager.SeedMfaPosture("id1", "as1", authTime), "an error is not cached as a failure")
	})
}

// Test_MfaDeleted_ResetsPosture locks in that removing a verified MFA enrollment, by the identity or an admin, fails
// MFA posture for the identity's api sessions, including ones seeded from a token.
func Test_MfaDeleted_ResetsPosture(t *testing.T) {
	manager, _ := newSeedTestManager()
	authTime := time.Now().Add(-time.Minute).UTC()
	mfaCheck := &PostureCheckMfa{TimeoutSeconds: PostureCheckNoTimeout}

	require.True(t, manager.SeedMfaPosture("id1", "as1", authTime))
	manager.SetMfaPosture("id1", "as2", true)

	manager.mfaDeleted(&db.Mfa{IdentityId: "id1", IsVerified: true})

	pd := manager.postureCache.PostureData("id1")
	require.False(t, mfaCheck.Evaluate("as1", pd))
	require.False(t, mfaCheck.Evaluate("as2", pd))
	require.False(t, manager.SeedMfaPosture("id1", "as1", authTime), "the token does not restore it")
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
