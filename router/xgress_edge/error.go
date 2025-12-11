package xgress_edge

import (
	"encoding/json"
	"fmt"

	"github.com/openziti/channel/v4"
	sdk "github.com/openziti/sdk-golang/ziti/edge"
)

var _ error = (*EdgeError)(nil)

type EdgeError struct {
	Message string `json:"message"`
	Code    uint32 `json:"code"`
	Cause   error  `json:"cause"`
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

// NewInvalidApiSessionTokenError creates a new EdgeError with the code ErrorInvalidApiSessionToken and the provided message.
func NewInvalidApiSessionTokenError(msg string) *EdgeError {
	return &EdgeError{Message: msg, Code: sdk.ErrorCodeInvalidApiSession}
}

// NewInvalidApiSessionType creates an EdgeError with the ErrorInvalidApiSessionType" code and the provided message.
func NewInvalidApiSessionType(msg string) *EdgeError {
	return &EdgeError{Message: msg, Code: sdk.ErrorCodeInvalidApiSessionType}
}
