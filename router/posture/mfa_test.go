package posture

import (
	"testing"
	"time"

	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
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
