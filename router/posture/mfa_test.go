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

// Test_MfaExpiresAt locks in that the pushed MFA expiry is the earliest applicable deadline, the
// MFA timeout or a pending wake/unlock grace deadline, matching what Evaluate enforces.
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

// Test_MarkMfaExpiryEnforced locks in that enforcement fires once per deadline: the same
// deadline reports new only the first time, while a moved deadline, from a shortened timeout or
// an advanced MFA pass, reports new again.
func Test_MarkMfaExpiryEnforced(t *testing.T) {
	instance := newInstance()
	deadline := time.Now().Add(-time.Minute)

	require.True(t, instance.MarkMfaExpiryEnforced(deadline), "a deadline never enforced before")
	require.False(t, instance.MarkMfaExpiryEnforced(deadline), "the same deadline is already enforced")
	require.True(t, instance.MarkMfaExpiryEnforced(deadline.Add(-time.Minute)), "an earlier deadline is a new one")
	require.True(t, instance.MarkMfaExpiryEnforced(deadline.Add(time.Hour)), "a later deadline is a new one")
}

func mfaPostureCheck(id string, timeoutSeconds int64, promptOnWake, promptOnUnlock bool) *edge_ctrl_pb.DataState_PostureCheck {
	return &edge_ctrl_pb.DataState_PostureCheck{
		Id:     id,
		Name:   id,
		TypeId: "MFA",
		Subtype: &edge_ctrl_pb.DataState_PostureCheck_Mfa_{
			Mfa: &edge_ctrl_pb.DataState_PostureCheck_Mfa{
				TimeoutSeconds: timeoutSeconds,
				PromptOnWake:   promptOnWake,
				PromptOnUnlock: promptOnUnlock,
			},
		},
	}
}

func macPostureCheck(id string) *edge_ctrl_pb.DataState_PostureCheck {
	return &edge_ctrl_pb.DataState_PostureCheck{
		Id:     id,
		Name:   id,
		TypeId: "MAC",
		Subtype: &edge_ctrl_pb.DataState_PostureCheck_Mac_{
			Mac: &edge_ctrl_pb.DataState_PostureCheck_Mac{MacAddresses: []string{"00:11:22:33:44:55"}},
		},
	}
}

// newPostureCheckRdm builds a router data model in which identityId belongs to a single dial
// policy carrying the given posture checks.
func newPostureCheckRdm(identityId string, checks ...*edge_ctrl_pb.DataState_PostureCheck) *common.RouterDataModel {
	rdm := common.NewBareRouterDataModel("r1")

	index := uint64(0)
	nextIndex := func() uint64 {
		index++
		return index
	}
	create := &edge_ctrl_pb.DataState_Event{Action: edge_ctrl_pb.DataState_Create}

	rdm.HandleIdentityEvent(nextIndex(), create, &edge_ctrl_pb.DataState_Event_Identity{
		Identity: &edge_ctrl_pb.DataState_Identity{Id: identityId, Name: identityId},
	})

	rdm.HandleServicePolicyEvent(nextIndex(), create, &edge_ctrl_pb.DataState_Event_ServicePolicy{
		ServicePolicy: &edge_ctrl_pb.DataState_ServicePolicy{
			Id:         "policy1",
			Name:       "policy1",
			PolicyType: edge_ctrl_pb.PolicyType_DialPolicy,
		},
	})

	var checkIds []string
	for _, check := range checks {
		rdm.HandlePostureCheckEvent(nextIndex(), create, &edge_ctrl_pb.DataState_Event_PostureCheck{PostureCheck: check})
		checkIds = append(checkIds, check.Id)
	}

	rdm.HandleServicePolicyChange(nextIndex(), &edge_ctrl_pb.DataState_ServicePolicyChange{
		PolicyId:          "policy1",
		RelatedEntityIds:  checkIds,
		RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedPostureCheck,
		Add:               true,
	})

	rdm.HandleServicePolicyChange(nextIndex(), &edge_ctrl_pb.DataState_ServicePolicyChange{
		PolicyId:          "policy1",
		RelatedEntityIds:  []string{identityId},
		RelatedEntityType: edge_ctrl_pb.ServicePolicyRelatedEntityType_RelatedIdentity,
		Add:               true,
	})

	rdm.SetCurrentIndex(index)
	return rdm
}

// Test_EarliestMfaExpiry_NoMfaCheck locks in that an identity governed only by non-MFA posture
// checks has no deadline: nothing about its access expires with the passage of time.
func Test_EarliestMfaExpiry_NoMfaCheck(t *testing.T) {
	rdm := newPostureCheckRdm("identity1", macPostureCheck("chk-mac"))

	require.Nil(t, EarliestMfaExpiry(rdm, "identity1", mfaState(time.Minute)))
}

// Test_EarliestMfaExpiry_UnknownIdentity locks in that an identity absent from the data model
// yields no deadline rather than an error or a panic.
func Test_EarliestMfaExpiry_UnknownIdentity(t *testing.T) {
	rdm := newPostureCheckRdm("identity1", mfaPostureCheck("chk-mfa", 3600, false, false))

	require.Nil(t, EarliestMfaExpiry(rdm, "identity2", mfaState(time.Minute)))
}

// Test_EarliestMfaExpiry_NoPostureData locks in that a session with no posture data, or one that
// has never passed MFA, yields no deadline: it is already failing, so there is nothing to expire.
func Test_EarliestMfaExpiry_NoPostureData(t *testing.T) {
	rdm := newPostureCheckRdm("identity1", mfaPostureCheck("chk-mfa", 3600, false, false))

	require.Nil(t, EarliestMfaExpiry(rdm, "identity1", nil), "no posture data: no deadline")
	require.Nil(t, EarliestMfaExpiry(rdm, "identity1", &InstanceData{}), "MFA never passed: no deadline")
}

// Test_EarliestMfaExpiry_FutureTimeout locks in that a still-valid MFA pass reports the timeout
// deadline it is heading for.
func Test_EarliestMfaExpiry_FutureTimeout(t *testing.T) {
	rdm := newPostureCheckRdm("identity1", mfaPostureCheck("chk-mfa", 3600, false, false))
	state := mfaState(time.Minute)

	expiresAt := EarliestMfaExpiry(rdm, "identity1", state)

	require.NotNil(t, expiresAt)
	require.WithinDuration(t, state.PassedMfaAt.Add(time.Hour), *expiresAt, time.Second)
	require.True(t, expiresAt.After(time.Now()), "the deadline has not been reached yet")
}

// Test_EarliestMfaExpiry_ElapsedTimeout locks in that an MFA pass past its timeout reports an
// elapsed deadline, which is what drives the router's time-based re-evaluation.
func Test_EarliestMfaExpiry_ElapsedTimeout(t *testing.T) {
	rdm := newPostureCheckRdm("identity1", mfaPostureCheck("chk-mfa", 60, false, false))
	state := mfaState(10 * time.Minute)

	expiresAt := EarliestMfaExpiry(rdm, "identity1", state)

	require.NotNil(t, expiresAt)
	require.WithinDuration(t, state.PassedMfaAt.Add(time.Minute), *expiresAt, time.Second)
	require.True(t, expiresAt.Before(time.Now()), "the deadline has elapsed")
}

// Test_EarliestMfaExpiry_ElapsedWakeGrace locks in that an expired post-wake re-prompt grace
// period reports an elapsed deadline even while the MFA timeout still has time left.
func Test_EarliestMfaExpiry_ElapsedWakeGrace(t *testing.T) {
	rdm := newPostureCheckRdm("identity1", mfaPostureCheck("chk-mfa", 3600, true, false))
	state := wokenAgo(mfaState(30*time.Minute), 10*time.Minute)
	wokenAt := state.Woken.Time.AsTime()

	expiresAt := EarliestMfaExpiry(rdm, "identity1", state)

	require.NotNil(t, expiresAt)
	require.WithinDuration(t, wokenAt.Add(PromptGracePeriod), *expiresAt, time.Second)
	require.True(t, expiresAt.Before(time.Now()), "the wake grace period has elapsed")
}

// Test_EarliestMfaExpiry_ElapsedUnlockGrace mirrors the wake case for unlock events.
func Test_EarliestMfaExpiry_ElapsedUnlockGrace(t *testing.T) {
	rdm := newPostureCheckRdm("identity1", mfaPostureCheck("chk-mfa", 3600, false, true))
	state := unlockedAgo(mfaState(30*time.Minute), 10*time.Minute)
	unlockedAt := state.Unlocked.Time.AsTime()

	expiresAt := EarliestMfaExpiry(rdm, "identity1", state)

	require.NotNil(t, expiresAt)
	require.WithinDuration(t, unlockedAt.Add(PromptGracePeriod), *expiresAt, time.Second)
	require.True(t, expiresAt.Before(time.Now()), "the unlock grace period has elapsed")
}

// Test_EarliestMfaExpiry_EarliestCheckWins locks in that the soonest deadline across every MFA
// check governing the identity is the one reported, so enforcement fires at the first expiry
// rather than the last.
func Test_EarliestMfaExpiry_EarliestCheckWins(t *testing.T) {
	rdm := newPostureCheckRdm("identity1",
		mfaPostureCheck("chk-long", 3600, false, false),
		mfaPostureCheck("chk-short", 600, false, false),
		macPostureCheck("chk-mac"))
	state := mfaState(time.Minute)

	expiresAt := EarliestMfaExpiry(rdm, "identity1", state)

	require.NotNil(t, expiresAt)
	require.WithinDuration(t, state.PassedMfaAt.Add(10*time.Minute), *expiresAt, time.Second)
}

// Test_EarliestMfaExpiry_UnboundedCheck locks in that an MFA check with no timeout and no pending
// re-prompt has no deadline, so it never triggers a re-evaluation.
func Test_EarliestMfaExpiry_UnboundedCheck(t *testing.T) {
	rdm := newPostureCheckRdm("identity1", mfaPostureCheck("chk-mfa", NoTimeout, true, true))

	require.Nil(t, EarliestMfaExpiry(rdm, "identity1", mfaState(time.Minute)))
}
