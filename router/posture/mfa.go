package posture

import (
	"fmt"
	"time"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/pkg/errors"
)

const (
	NoTimeout         = int64(-1)
	PromptGracePeriod = 5 * time.Minute
)

type MfaCheck struct {
	*edge_ctrl_pb.DataState_PostureCheck
	*edge_ctrl_pb.DataState_PostureCheck_Mfa
}

// timeoutDeadline returns when the MFA timeout expires the check, or nil when the check has no
// timeout. state.PassedMfaAt must be non-nil.
func (m *MfaCheck) timeoutDeadline(state *InstanceData) *time.Time {
	if m.TimeoutSeconds < 0 {
		return nil
	}
	deadline := state.PassedMfaAt.Add(time.Duration(m.TimeoutSeconds) * time.Second)
	return &deadline
}

// wakeDeadline returns when the post-wake re-prompt grace period expires the check: set only when
// the endpoint reported a wake event that MFA has not been re-passed since. No wake event reported,
// or an MFA pass after the wake, yields no deadline. state.PassedMfaAt must be non-nil.
func (m *MfaCheck) wakeDeadline(state *InstanceData) *time.Time {
	if !m.PromptOnWake || state.Woken == nil {
		return nil
	}
	wokenAt := state.Woken.Time.AsTime()
	if !state.PassedMfaAt.Before(wokenAt) {
		return nil
	}
	deadline := wokenAt.Add(PromptGracePeriod)
	return &deadline
}

// unlockDeadline mirrors wakeDeadline for unlock events. state.PassedMfaAt must be non-nil.
func (m *MfaCheck) unlockDeadline(state *InstanceData) *time.Time {
	if !m.PromptOnUnlock || state.Unlocked == nil {
		return nil
	}
	unlockedAt := state.Unlocked.Time.AsTime()
	if !state.PassedMfaAt.Before(unlockedAt) {
		return nil
	}
	deadline := unlockedAt.Add(PromptGracePeriod)
	return &deadline
}

func (m *MfaCheck) Evaluate(state *InstanceData) *CheckError {
	now := time.Now()

	if state == nil {
		return &CheckError{
			Id:    m.Id,
			Name:  m.Name,
			Cause: NilStateError,
		}
	}

	if state.PassedMfaAt == nil {
		return &CheckError{
			Id:    m.Id,
			Name:  m.Name,
			Cause: errors.New("MFA has never been passed"),
		}
	}

	if deadline := m.timeoutDeadline(state); deadline != nil && now.After(*deadline) {
		timeout := time.Duration(m.TimeoutSeconds) * time.Second
		return &CheckError{
			Id:    m.Id,
			Name:  m.Name,
			Cause: fmt.Errorf("last MFA check exceeded timeout of %s, last mfa at %s, checked at %s", timeout.String(), state.PassedMfaAt.String(), now.String()),
		}
	}

	if deadline := m.wakeDeadline(state); deadline != nil && now.After(*deadline) {
		return &CheckError{
			Id:    m.Id,
			Name:  m.Name,
			Cause: fmt.Errorf("MFA not resupplied during grace period, woken at %s, grace period: %s, supplied at %s, checked at: %s", state.Woken.Time.AsTime().String(), PromptGracePeriod.String(), state.PassedMfaAt.String(), now.String()),
		}
	}

	if deadline := m.unlockDeadline(state); deadline != nil && now.After(*deadline) {
		return &CheckError{
			Id:    m.Id,
			Name:  m.Name,
			Cause: fmt.Errorf("MFA not resupplied during grace period, unlocked at %s, grace period: %s, supplied at %s, checked at: %s", state.Unlocked.Time.AsTime().String(), PromptGracePeriod.String(), state.PassedMfaAt.String(), now.String()),
		}
	}

	return nil
}

// MfaExpiresAt returns the earliest moment at which the MFA check will stop passing given the
// current state: the soonest of the MFA timeout and any pending wake/unlock re-prompt grace
// deadlines. Returns nil when MFA has never passed or when nothing bounds the current pass. It
// shares the deadline computations with MfaCheck.Evaluate so a pushed expiry always matches the
// evaluation outcome.
func MfaExpiresAt(mfa *edge_ctrl_pb.DataState_PostureCheck_Mfa, state *InstanceData) *time.Time {
	if mfa == nil || state == nil || state.PassedMfaAt == nil {
		return nil
	}

	m := &MfaCheck{DataState_PostureCheck_Mfa: mfa}

	var expiresAt *time.Time
	for _, deadline := range []*time.Time{m.timeoutDeadline(state), m.wakeDeadline(state), m.unlockDeadline(state)} {
		if deadline != nil && (expiresAt == nil || deadline.Before(*expiresAt)) {
			expiresAt = deadline
		}
	}
	return expiresAt
}
