package posture

import (
	"testing"
	"time"

	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newMfaCheck(timeoutSeconds int64, promptOnWake, promptOnUnlock bool) *MfaCheck {
	return &MfaCheck{
		DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{Id: "mfa-check", Name: "mfa-check"},
		DataState_PostureCheck_Mfa: &edge_ctrl_pb.DataState_PostureCheck_Mfa{
			TimeoutSeconds: timeoutSeconds,
			PromptOnWake:   promptOnWake,
			PromptOnUnlock: promptOnUnlock,
		},
	}
}

func mfaState(passedMfaAgo time.Duration) *InstanceData {
	passedAt := time.Now().Add(-passedMfaAgo)
	return &InstanceData{PassedMfaAt: &passedAt}
}

func wokenAgo(state *InstanceData, ago time.Duration) *InstanceData {
	state.Woken = &edge_client_pb.PostureResponse_Woken{Time: timestamppb.New(time.Now().Add(-ago))}
	return state
}

func unlockedAgo(state *InstanceData, ago time.Duration) *InstanceData {
	state.Unlocked = &edge_client_pb.PostureResponse_Unlocked{Time: timestamppb.New(time.Now().Add(-ago))}
	return state
}

// Test_MfaCheck_PromptOnWake_NoWakeEvent locks in that a PromptOnWake check with no wake event
// reported passes rather than dereferencing the nil Woken state.
func Test_MfaCheck_PromptOnWake_NoWakeEvent(t *testing.T) {
	check := newMfaCheck(NoTimeout, true, false)

	require.Nil(t, check.Evaluate(mfaState(time.Minute)), "no wake event reported: nothing to re-prompt for")
}

// Test_MfaCheck_PromptOnUnlock_NoUnlockEvent locks in the same for unlock events.
func Test_MfaCheck_PromptOnUnlock_NoUnlockEvent(t *testing.T) {
	check := newMfaCheck(NoTimeout, false, true)

	require.Nil(t, check.Evaluate(mfaState(time.Minute)), "no unlock event reported: nothing to re-prompt for")
}

// Test_MfaCheck_PromptOnWake_RepassSatisfies locks in that passing MFA after a wake event
// satisfies the re-prompt: the check must not fail forever once the wake grace period has passed.
func Test_MfaCheck_PromptOnWake_RepassSatisfies(t *testing.T) {
	check := newMfaCheck(NoTimeout, true, false)
	state := wokenAgo(mfaState(time.Minute), time.Hour)

	require.Nil(t, check.Evaluate(state), "MFA re-passed after the wake: the re-prompt is satisfied")
}

// Test_MfaCheck_PromptOnWake_GraceExpired locks in that a wake event without a subsequent MFA
// pass fails the check once the grace period elapses.
func Test_MfaCheck_PromptOnWake_GraceExpired(t *testing.T) {
	check := newMfaCheck(NoTimeout, true, false)
	state := wokenAgo(mfaState(time.Hour), 10*time.Minute)

	require.NotNil(t, check.Evaluate(state), "no MFA re-pass within the wake grace period fails the check")
}

// Test_MfaCheck_PromptOnWake_WithinGrace locks in that the check keeps passing during the wake
// grace period, giving the user time to re-prompt.
func Test_MfaCheck_PromptOnWake_WithinGrace(t *testing.T) {
	check := newMfaCheck(NoTimeout, true, false)
	state := wokenAgo(mfaState(time.Hour), time.Minute)

	require.Nil(t, check.Evaluate(state), "within the wake grace period the check still passes")
}

// Test_MfaCheck_PromptOnUnlock_RepassSatisfies mirrors the wake re-pass semantics for unlocks.
func Test_MfaCheck_PromptOnUnlock_RepassSatisfies(t *testing.T) {
	check := newMfaCheck(NoTimeout, false, true)
	state := unlockedAgo(mfaState(time.Minute), time.Hour)

	require.Nil(t, check.Evaluate(state), "MFA re-passed after the unlock: the re-prompt is satisfied")
}

// Test_MfaCheck_Timeout locks in the plain MFA timeout evaluation.
func Test_MfaCheck_Timeout(t *testing.T) {
	check := newMfaCheck(60, false, false)

	require.Nil(t, check.Evaluate(mfaState(10*time.Second)), "within the MFA timeout")
	require.NotNil(t, check.Evaluate(mfaState(2*time.Minute)), "past the MFA timeout")
}

func seedClaims(apiSessionId string, amr []string, authTime, issuedAt time.Time) *common.AccessClaims {
	claims := &common.AccessClaims{}
	claims.ApiSessionId = apiSessionId
	claims.AuthenticationMethodsReferences = amr
	if !authTime.IsZero() {
		claims.AuthTime = oidc.FromTime(authTime)
	}
	claims.TokenClaims.IssuedAt = oidc.FromTime(issuedAt)
	return claims
}

// Test_SeedMfaFromApiSession_TotpAuthSeedsBaseline locks in that an api session token whose amr
// includes totp establishes the MFA-passed baseline (from auth_time) without any posture TOTP
// token, so router-enforced MFA checks pass for a session that authenticated with TOTP.
func Test_SeedMfaFromApiSession_TotpAuthSeedsBaseline(t *testing.T) {
	cache := NewCache(nil)
	authTime := time.Now().Add(-time.Minute)

	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"password", "totp"}, authTime, time.Now()))

	instance := cache.GetInstance("as1")
	require.NotNil(t, instance, "seeding must create the posture instance for the session")
	require.NotNil(t, instance.PassedMfaAt)
	require.WithinDuration(t, authTime, *instance.PassedMfaAt, time.Second, "baseline comes from auth_time")

	check := newMfaCheck(3600, true, true)
	require.Nil(t, check.Evaluate(&instance.InstanceData), "MFA check passes off the seeded baseline")
}

// Test_SeedMfaFromApiSession_NoTotpAmrDoesNotSeed locks in that a session that never passed TOTP
// establishes no baseline: MFA checks keep failing until a posture TOTP token arrives.
func Test_SeedMfaFromApiSession_NoTotpAmrDoesNotSeed(t *testing.T) {
	cache := NewCache(nil)

	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"password"}, time.Now(), time.Now()))

	instance := cache.GetInstance("as1")
	if instance != nil {
		require.Nil(t, instance.PassedMfaAt, "no totp amr: no MFA baseline")
	}
}

// Test_SeedMfaFromApiSession_NeverOverwrites locks in that seeding never moves an existing
// MFA-passed time: a refreshed api session token must not extend an MFA window, and a
// posture-response TOTP token always wins over the session baseline.
func Test_SeedMfaFromApiSession_NeverOverwrites(t *testing.T) {
	cache := NewCache(nil)
	firstAuth := time.Now().Add(-time.Hour)

	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"totp"}, firstAuth, time.Now()))
	// A later token (e.g. a refresh) with a newer auth_time must not advance the baseline.
	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"totp"}, time.Now(), time.Now()))

	instance := cache.GetInstance("as1")
	require.NotNil(t, instance)
	require.NotNil(t, instance.PassedMfaAt)
	require.WithinDuration(t, firstAuth, *instance.PassedMfaAt, time.Second, "seed is set once, never advanced")
}

// Test_SeedMfaFromApiSession_NoAuthTimeDoesNotSeed locks in that a token without auth_time seeds
// nothing. There is no iat fallback: iat moves on every refresh, so a router meeting the session
// late would seed a baseline newer than the real TOTP pass and extend the MFA window. OpenZiti
// mints its OIDC tokens and always sets auth_time; a token without it gets no MFA baseline.
func Test_SeedMfaFromApiSession_NoAuthTimeDoesNotSeed(t *testing.T) {
	cache := NewCache(nil)

	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"totp"}, time.Time{}, time.Now()))

	instance := cache.GetInstance("as1")
	if instance != nil {
		require.Nil(t, instance.PassedMfaAt, "no auth_time: no MFA baseline; iat must never be used")
	}
}

// Test_SeedMfaFromApiSession_NilClaims locks in that nil claims are a no-op.
func Test_SeedMfaFromApiSession_NilClaims(t *testing.T) {
	cache := NewCache(nil)

	cache.SeedMfaFromApiSession("id1", "as1", nil)

	require.Nil(t, cache.GetInstance("as1"))
}

// Test_MfaExpiresAt locks in that the pushed MFA expiry is the earliest applicable deadline —
// the MFA timeout and any pending wake/unlock grace deadlines — matching what Evaluate enforces.
func Test_MfaExpiresAt(t *testing.T) {
	mfa := &edge_ctrl_pb.DataState_PostureCheck_Mfa{TimeoutSeconds: 3600, PromptOnWake: true}

	require.Nil(t, MfaExpiresAt(mfa, nil), "no state: no deadline")
	require.Nil(t, MfaExpiresAt(mfa, &InstanceData{}), "MFA never passed: no deadline")

	// Only the timeout applies when no wake is pending.
	state := mfaState(time.Minute)
	expiresAt := MfaExpiresAt(mfa, state)
	require.NotNil(t, expiresAt)
	require.WithinDuration(t, state.PassedMfaAt.Add(time.Hour), *expiresAt, time.Second)

	// A pending wake's grace deadline is sooner than the timeout and must win: MFA passed 10
	// minutes ago (timeout deadline ~50 minutes out), woken 1 minute ago (grace deadline 4
	// minutes out).
	state = wokenAgo(mfaState(10*time.Minute), time.Minute)
	wokenAt := state.Woken.Time.AsTime()
	expiresAt = MfaExpiresAt(mfa, state)
	require.NotNil(t, expiresAt)
	require.WithinDuration(t, wokenAt.Add(PromptGracePeriod), *expiresAt, time.Second)

	// A re-pass after the wake clears the grace deadline, leaving only the timeout.
	state = wokenAgo(mfaState(time.Minute), time.Hour)
	expiresAt = MfaExpiresAt(mfa, state)
	require.NotNil(t, expiresAt)
	require.WithinDuration(t, state.PassedMfaAt.Add(time.Hour), *expiresAt, time.Second)

	// No timeout and nothing pending: no deadline at all.
	noTimeout := &edge_ctrl_pb.DataState_PostureCheck_Mfa{TimeoutSeconds: NoTimeout}
	require.Nil(t, MfaExpiresAt(noTimeout, mfaState(time.Minute)))
}
