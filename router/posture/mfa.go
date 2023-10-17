package posture

import (
	"fmt"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/pkg/errors"
	"time"
)

const (
	NoTimeout         = int64(-1)
	PromptGracePeriod = 5 * time.Minute
)

type MfaCheck struct {
	*edge_ctrl_pb.DataState_PostureCheck
	*edge_ctrl_pb.DataState_PostureCheck_Mfa
}

func (m *MfaCheck) Evaluate(state *Cache) *CheckError {
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

	if m.TimeoutSeconds != NoTimeout {
		timeout := time.Duration(m.TimeoutSeconds) * time.Second
		timedOut := state.PassedMfaAt.Add(timeout).Before(time.Now())

		if timedOut {
			return &CheckError{
				Id:    m.Id,
				Name:  m.Name,
				Cause: fmt.Errorf("last MFA check exceeded timeout of %s, last mfa at %s, checked at %s", timeout.String(), state.PassedMfaAt.String(), now.String()),
			}
		}
	}

	if m.PromptOnWake {
		wokenAt := state.Woken.Time.AsTime()
		wokenGraceEndsAt := wokenAt.Add(PromptGracePeriod)

		if now.After(wokenGraceEndsAt) {
			return &CheckError{
				Id:    m.Id,
				Name:  m.Name,
				Cause: fmt.Errorf("MFA not resupplied during grace period, woken at %s, grace period: %s, supplied at %s, checked at: %s", wokenAt.String(), PromptGracePeriod.String(), state.PassedMfaAt.String(), now.String()),
			}
		}
	}

	if m.PromptOnUnlock {
		unlockedAt := state.Unlocked.Time.AsTime()
		unlockedGraceEndsAt := unlockedAt.Add(PromptGracePeriod)

		if now.After(unlockedGraceEndsAt) {
			return &CheckError{
				Id:    m.Id,
				Name:  m.Name,
				Cause: fmt.Errorf("MFA not resupplied during grace period, unlocked at %s, grace period: %s, supplied at %s, checked at: %s", unlockedAt.String(), PromptGracePeriod.String(), state.PassedMfaAt.String(), now.String()),
			}
		}
	}

	return nil
}
