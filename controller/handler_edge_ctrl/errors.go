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

package handler_edge_ctrl

import (
	"errors"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/apierror"
)

type controllerError interface {
	error
	ErrorCode() uint32
	GetRetryHint() edge.RetryHint
}

func retryHintToResult(hint edge.RetryHint) edge_ctrl_pb.CreateTerminatorResult {
	switch hint {
	case edge.RetryTooBusy:
		return edge_ctrl_pb.CreateTerminatorResult_FailedBusy
	default:
		return edge_ctrl_pb.CreateTerminatorResult_FailedOther
	}
}

func internalError(err error) controllerError {
	if err == nil {
		return nil
	}
	var ctrlErr controllerError
	if errors.As(err, &ctrlErr) {
		return ctrlErr
	}
	// A "cluster has no leader" condition is transient: it happens when a model update is
	// attempted before raft leadership is established (notably during controller startup, when
	// a router tries to create its tunnel terminator before the controller wins the election).
	// Classify it as busy (RetryTooBusy -> FailedBusy) so callers like terminator establishment
	// back off and retry instead of giving up, which would otherwise strand the service.
	if isClusterNoLeaderErr(err) {
		return busyError(err)
	}
	return internalErrorWrapper{error: err}
}

func isClusterNoLeaderErr(err error) bool {
	var apiErr *errorz.ApiError
	if errors.As(err, &apiErr) {
		return apiErr.AppCode == apierror.ClusterHasNoLeaderCode
	}
	return false
}

func nonRetriableError(err error) controllerError {
	if err == nil {
		return nil
	}
	return nonRetriableErrorWrapper{error: err}
}

type internalErrorWrapper struct {
	error
}

func (internalErrorWrapper) ErrorCode() uint32 {
	return edge.ErrorCodeInternal
}

func (internalErrorWrapper) GetRetryHint() edge.RetryHint {
	return edge.RetryDefault
}

func busyError(err error) controllerError {
	if err == nil {
		return nil
	}
	return &genericControllerError{
		Message:   err.Error(),
		Code:      edge.ErrorCodeInternal,
		RetryHint: edge.RetryTooBusy,
	}
}

type nonRetriableErrorWrapper struct {
	error
}

func (nonRetriableErrorWrapper) ErrorCode() uint32 {
	return edge.ErrorCodeInternal
}

func (nonRetriableErrorWrapper) GetRetryHint() edge.RetryHint {
	return edge.RetryNotRetriable
}

type InvalidApiSessionError struct{}

func (InvalidApiSessionError) Error() string {
	return "invalid api session"
}

func (self InvalidApiSessionError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidApiSession
}

func (InvalidApiSessionError) GetRetryHint() edge.RetryHint {
	return edge.RetryStartOver
}

type InvalidSessionError struct{}

func (InvalidSessionError) Error() string {
	return "invalid session"
}

func (self InvalidSessionError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidSession
}

func (InvalidSessionError) GetRetryHint() edge.RetryHint {
	return edge.RetryStartOver
}

type WrongSessionTypeError struct{}

func (WrongSessionTypeError) Error() string {
	return "incorrect session type"
}

func (self WrongSessionTypeError) ErrorCode() uint32 {
	return edge.ErrorCodeWrongSessionType
}

func (WrongSessionTypeError) GetRetryHint() edge.RetryHint {
	return edge.RetryStartOver
}

type InvalidEdgeRouterForSessionError struct{}

func (InvalidEdgeRouterForSessionError) Error() string {
	return "invalid edge router for session"
}

func (self InvalidEdgeRouterForSessionError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidEdgeRouterForSession
}

func (InvalidEdgeRouterForSessionError) GetRetryHint() edge.RetryHint {
	return edge.RetryStartOver
}

type InvalidServiceError struct{}

func (InvalidServiceError) Error() string {
	return "invalid service"
}

func (self InvalidServiceError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidService
}

func (InvalidServiceError) GetRetryHint() edge.RetryHint {
	return edge.RetryNotRetriable
}

type TunnelingNotEnabledError struct{}

func (TunnelingNotEnabledError) Error() string {
	return "tunneling not enabled"
}

func (self TunnelingNotEnabledError) ErrorCode() uint32 {
	return edge.ErrorCodeTunnelingNotEnabled
}

func (TunnelingNotEnabledError) GetRetryHint() edge.RetryHint {
	return edge.RetryNotRetriable
}

func invalidTerminator(msg string) controllerError {
	return &genericControllerError{
		Message:   msg,
		Code:      edge.ErrorCodeInvalidTerminator,
		RetryHint: edge.RetryNotRetriable,
	}
}

func invalidCost(msg string) controllerError {
	return &genericControllerError{
		Message:   msg,
		Code:      edge.ErrorCodeInvalidCost,
		RetryHint: edge.RetryNotRetriable,
	}
}

func invalidPrecedence(msg string) controllerError {
	return &genericControllerError{
		Message:   msg,
		Code:      edge.ErrorCodeInvalidPrecedence,
		RetryHint: edge.RetryNotRetriable,
	}
}

func encryptionDataMissing(msg string) controllerError {
	return &genericControllerError{
		Message:   msg,
		Code:      edge.ErrorCodeEncryptionDataMissing,
		RetryHint: edge.RetryStartOver,
	}
}

type genericControllerError struct {
	Message   string
	Code      uint32
	RetryHint edge.RetryHint
}

func (self *genericControllerError) Error() string {
	return self.Message
}

func (self *genericControllerError) ErrorCode() uint32 {
	return self.Code
}

func (self *genericControllerError) GetRetryHint() edge.RetryHint {
	return self.RetryHint
}
