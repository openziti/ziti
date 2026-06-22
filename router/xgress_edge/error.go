package xgress_edge

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/openziti/channel/v5"
	sdk "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/router/posture"
	"github.com/openziti/ziti/v2/router/state"
)

var _ error = (*EdgeError)(nil)

type EdgeError struct {
	Message string `json:"message"`
	Code    uint32 `json:"code"`
	// Cause never rides the wire: a non-nil error marshals as a JSON object the SDK cannot
	// unmarshal into its error-typed field, which would discard the whole structured error.
	// Constructors put the cause's text into Message.
	Cause     error         `json:"-"`
	RetryHint sdk.RetryHint `json:"retryHint"`
	// FailingPostureCheckIds carries the ids of the posture checks that denied access when Code
	// is ErrorCodePostureCheckFailed. Field name matches the SDK's edge.Error.
	FailingPostureCheckIds []string `json:"failingPostureCheckIds,omitempty"`
}

func (e EdgeError) Error() string {
	ret := fmt.Errorf("code: %d, message: %s", e.Code, e.Message).Error()

	if e.Cause != nil {
		ret += fmt.Sprintf(", Cause: %v", e.Cause)
	}

	return ret
}

func (e EdgeError) Unwrap() error {
	return e.Cause
}

func (e EdgeError) ApplyToMsg(msg *channel.Message) {
	errAsJson, err := json.Marshal(e)

	if err != nil {
		return
	}
	if len(errAsJson) > 0 {
		msg.Headers.PutStringHeader(sdk.StructuredError, string(errAsJson))
		msg.Headers.PutUint32Header(sdk.ErrorCodeHeader, e.Code)
	}
}

// NewAccessDeniedError classifies an access-check failure into a structured EdgeError: posture
// denials (policies grant access but checks fail) carry ErrorCodePostureCheckFailed plus the
// failing check ids; everything else — including no granting policies — is a plain access denial.
func NewAccessDeniedError(err error) *EdgeError {
	edgeErr := &EdgeError{
		Message: err.Error(),
		Code:    sdk.ErrorCodeAccessDenied,
		Cause:   err,
	}

	var policyErrs *posture.PolicyAccessErrors
	if errors.As(err, &policyErrs) {
		edgeErr.Code = sdk.ErrorCodePostureCheckFailed
		seen := map[string]struct{}{}
		for _, policyErr := range *policyErrs {
			for _, checkId := range policyErr.FailingCheckIds {
				if _, dup := seen[checkId]; !dup {
					seen[checkId] = struct{}{}
					edgeErr.FailingPostureCheckIds = append(edgeErr.FailingPostureCheckIds, checkId)
				}
			}
		}
		sort.Strings(edgeErr.FailingPostureCheckIds)
	}

	return edgeErr
}

// NewServiceSessionDenialError classifies a service-session validation failure: policy and
// posture denials map exactly as NewAccessDeniedError does, a service id unknown to the data
// model is an invalid service, and anything else (parse failures, expired or mistyped tokens)
// is an invalid session.
func NewServiceSessionDenialError(err error) *EdgeError {
	var policyErrs *posture.PolicyAccessErrors
	var noPolicies *posture.NoPoliciesError
	if errors.As(err, &policyErrs) || errors.As(err, &noPolicies) {
		return NewAccessDeniedError(err)
	}
	if errors.Is(err, state.ErrServiceNotFound) {
		return &EdgeError{Message: err.Error(), Code: sdk.ErrorCodeInvalidService, Cause: err}
	}
	return &EdgeError{Message: err.Error(), Code: sdk.ErrorCodeInvalidSession, Cause: err}
}

// NewInvalidApiSessionTokenError creates a new EdgeError with the code ErrorInvalidApiSessionToken and the provided message.
func NewInvalidApiSessionTokenError(msg string) *EdgeError {
	return &EdgeError{Message: msg, Code: sdk.ErrorCodeInvalidApiSession}
}

// NewInvalidApiSessionType creates an EdgeError with the ErrorInvalidApiSessionType" code and the provided message.
func NewInvalidApiSessionType(msg string) *EdgeError {
	return &EdgeError{Message: msg, Code: sdk.ErrorCodeInvalidApiSessionType}
}
