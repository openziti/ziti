package posture

import (
	"testing"
	"time"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newMfaCheck(promptOnWake, promptOnUnlock bool) *MfaCheck {
	return &MfaCheck{
		DataState_PostureCheck: &edge_ctrl_pb.DataState_PostureCheck{
			Id:   "test-mfa-id",
			Name: "my-mfa-check",
		},
		DataState_PostureCheck_Mfa: &edge_ctrl_pb.DataState_PostureCheck_Mfa{
			TimeoutSeconds: NoTimeout,
			PromptOnWake:   promptOnWake,
			PromptOnUnlock: promptOnUnlock,
		},
	}
}

func newMfaInstanceData(passedMfaAt time.Time) *InstanceData {
	return &InstanceData{
		IdentityId:   "test-identity",
		ApiSessionId: "test-api-session",
		PassedMfaAt:  &passedMfaAt,
	}
}

// MFA passed, promptOnWake enabled, but no wake event has ever been reported: Woken is nil.
// Must pass without panicking, matching the controller's PassedOnWake semantics.
func TestMfaCheck_PromptOnWake_NilWoken_Passes(t *testing.T) {
	check := newMfaCheck(true, false)
	data := newMfaInstanceData(time.Now().Add(-time.Minute))

	if result := check.Evaluate(data); result != nil {
		t.Fatalf("expected pass when promptOnWake is set and no wake event was reported, got: %v", result)
	}
}

// MFA passed, promptOnUnlock enabled, but no unlock event has ever been reported: Unlocked is nil.
// Must pass without panicking, matching the controller's PassedOnUnlock semantics.
func TestMfaCheck_PromptOnUnlock_NilUnlocked_Passes(t *testing.T) {
	check := newMfaCheck(false, true)
	data := newMfaInstanceData(time.Now().Add(-time.Minute))

	if result := check.Evaluate(data); result != nil {
		t.Fatalf("expected pass when promptOnUnlock is set and no unlock event was reported, got: %v", result)
	}
}

// Both prompts enabled, neither event reported. Must pass without panicking.
func TestMfaCheck_BothPrompts_NilEvents_Passes(t *testing.T) {
	check := newMfaCheck(true, true)
	data := newMfaInstanceData(time.Now().Add(-time.Minute))

	if result := check.Evaluate(data); result != nil {
		t.Fatalf("expected pass when no wake/unlock events were reported, got: %v", result)
	}
}

// A wake event that occurred before MFA was last passed is satisfied by that MFA pass.
func TestMfaCheck_PromptOnWake_WokenBeforeMfa_Passes(t *testing.T) {
	check := newMfaCheck(true, false)
	data := newMfaInstanceData(time.Now().Add(-time.Minute))
	data.Woken = &edge_client_pb.PostureResponse_Woken{
		Time: timestamppb.New(time.Now().Add(-time.Hour)),
	}

	if result := check.Evaluate(data); result != nil {
		t.Fatalf("expected pass when wake event predates the MFA pass, got: %v", result)
	}
}

// An unlock event that occurred before MFA was last passed is satisfied by that MFA pass.
func TestMfaCheck_PromptOnUnlock_UnlockedBeforeMfa_Passes(t *testing.T) {
	check := newMfaCheck(false, true)
	data := newMfaInstanceData(time.Now().Add(-time.Minute))
	data.Unlocked = &edge_client_pb.PostureResponse_Unlocked{
		Time: timestamppb.New(time.Now().Add(-time.Hour)),
	}

	if result := check.Evaluate(data); result != nil {
		t.Fatalf("expected pass when unlock event predates the MFA pass, got: %v", result)
	}
}

// A wake event after the last MFA pass, still within the grace period, passes.
func TestMfaCheck_PromptOnWake_WokenAfterMfaWithinGrace_Passes(t *testing.T) {
	check := newMfaCheck(true, false)
	data := newMfaInstanceData(time.Now().Add(-time.Hour))
	data.Woken = &edge_client_pb.PostureResponse_Woken{
		Time: timestamppb.New(time.Now().Add(-time.Minute)),
	}

	if result := check.Evaluate(data); result != nil {
		t.Fatalf("expected pass for wake event within the grace period, got: %v", result)
	}
}

// A wake event after the last MFA pass, beyond the grace period, fails.
func TestMfaCheck_PromptOnWake_WokenAfterMfaBeyondGrace_Fails(t *testing.T) {
	check := newMfaCheck(true, false)
	data := newMfaInstanceData(time.Now().Add(-time.Hour))
	data.Woken = &edge_client_pb.PostureResponse_Woken{
		Time: timestamppb.New(time.Now().Add(-10 * time.Minute)),
	}

	if result := check.Evaluate(data); result == nil {
		t.Fatal("expected failure for wake event beyond the grace period without MFA resupply, got pass")
	}
}

// An unlock event after the last MFA pass, beyond the grace period, fails.
func TestMfaCheck_PromptOnUnlock_UnlockedAfterMfaBeyondGrace_Fails(t *testing.T) {
	check := newMfaCheck(false, true)
	data := newMfaInstanceData(time.Now().Add(-time.Hour))
	data.Unlocked = &edge_client_pb.PostureResponse_Unlocked{
		Time: timestamppb.New(time.Now().Add(-10 * time.Minute)),
	}

	if result := check.Evaluate(data); result == nil {
		t.Fatal("expected failure for unlock event beyond the grace period without MFA resupply, got pass")
	}
}

// MFA never passed still fails regardless of prompt flags.
func TestMfaCheck_NeverPassedMfa_Fails(t *testing.T) {
	check := newMfaCheck(true, true)
	data := &InstanceData{
		IdentityId:   "test-identity",
		ApiSessionId: "test-api-session",
	}

	if result := check.Evaluate(data); result == nil {
		t.Fatal("expected failure when MFA has never been passed, got pass")
	}
}

// EvaluatePostureCheck must convert an evaluation panic into a failed check, never
// propagate it to the caller. An unknown subtype produces a nil Checker whose
// Evaluate call panics.
func TestEvaluatePostureCheck_PanicBecomesFailedCheck(t *testing.T) {
	postureCheck := &edge_ctrl_pb.DataState_PostureCheck{
		Id:   "test-unknown-id",
		Name: "my-unknown-check",
	}

	result := EvaluatePostureCheck(postureCheck, &InstanceData{})

	if result == nil {
		t.Fatal("expected a non-nil CheckError for an unevaluable posture check, got nil")
	}
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

// Test_SeedMfaFromApiSession_TotpAuthSeedsBaseline verifies a TOTP-attested token sets the
// MFA-passed time from auth_time.
func Test_SeedMfaFromApiSession_TotpAuthSeedsBaseline(t *testing.T) {
	cache := NewCache(nil)
	authTime := time.Now().Add(-time.Minute)

	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"password", "totp"}, authTime, time.Now()))

	instance := cache.GetInstance("as1")
	require.NotNil(t, instance, "seeding must create the posture instance for the session")
	require.NotNil(t, instance.PassedMfaAt)
	require.WithinDuration(t, authTime, *instance.PassedMfaAt, time.Second, "baseline comes from auth_time")

	check := newMfaCheck(true, true)
	check.TimeoutSeconds = 3600
	require.Nil(t, check.Evaluate(&instance.InstanceData), "MFA check passes off the seeded baseline")
}

// Test_SeedMfaFromApiSession_NoTotpAmrDoesNotSeed verifies a token without totp in amr seeds nothing.
func Test_SeedMfaFromApiSession_NoTotpAmrDoesNotSeed(t *testing.T) {
	cache := NewCache(nil)

	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"password"}, time.Now(), time.Now()))

	instance := cache.GetInstance("as1")
	if instance != nil {
		require.Nil(t, instance.PassedMfaAt, "no totp amr: no MFA baseline")
	}
}

// Test_SeedMfaFromApiSession_NeverOverwrites verifies seeding never moves an existing MFA-passed
// time, so a refreshed token cannot extend the MFA window.
func Test_SeedMfaFromApiSession_NeverOverwrites(t *testing.T) {
	cache := NewCache(nil)
	firstAuth := time.Now().Add(-time.Hour)

	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"totp"}, firstAuth, time.Now()))
	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"totp"}, time.Now(), time.Now()))

	instance := cache.GetInstance("as1")
	require.NotNil(t, instance)
	require.NotNil(t, instance.PassedMfaAt)
	require.WithinDuration(t, firstAuth, *instance.PassedMfaAt, time.Second, "seed is set once, never advanced")
}

// Test_SeedMfaFromApiSession_NoAuthTimeDoesNotSeed verifies a token without auth_time seeds nothing.
// iat is not a fallback because it moves on every refresh.
func Test_SeedMfaFromApiSession_NoAuthTimeDoesNotSeed(t *testing.T) {
	cache := NewCache(nil)

	cache.SeedMfaFromApiSession("id1", "as1", seedClaims("as1", []string{"totp"}, time.Time{}, time.Now()))

	instance := cache.GetInstance("as1")
	if instance != nil {
		require.Nil(t, instance.PassedMfaAt, "no auth_time: no MFA baseline; iat must never be used")
	}
}

// Test_SeedMfaFromApiSession_NilClaims verifies nil claims are a no-op.
func Test_SeedMfaFromApiSession_NilClaims(t *testing.T) {
	cache := NewCache(nil)

	cache.SeedMfaFromApiSession("id1", "as1", nil)

	require.Nil(t, cache.GetInstance("as1"))
}
