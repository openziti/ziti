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

	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
)

type controllerError interface {
	error
	ErrorCode() uint32
	GetRetryType() edge_ctrl_pb.CreateTerminatorResult
}

func internalError(err error) controllerError {
	if err == nil {
		return nil
	}
	var ctrlErr controllerError
	if errors.As(err, &ctrlErr) {
		return ctrlErr
	}
	return internalErrorWrapper{error: err}
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

func (internalErrorWrapper) GetRetryType() edge_ctrl_pb.CreateTerminatorResult {
	return edge_ctrl_pb.CreateTerminatorResult_FailedOther
}

func busyError(err error) controllerError {
	if err == nil {
		return nil
	}
	return &genericControllerError{
		msg:       err.Error(),
		errorCode: edge.ErrorCodeInternal,
		retryType: edge_ctrl_pb.CreateTerminatorResult_FailedBusy,
	}
}

type nonRetriableErrorWrapper struct {
	error
}

func (nonRetriableErrorWrapper) ErrorCode() uint32 {
	return edge.ErrorCodeInternal
}

func (nonRetriableErrorWrapper) GetRetryType() edge_ctrl_pb.CreateTerminatorResult {
	return edge_ctrl_pb.CreateTerminatorResult_FailedPermanent
}

type InvalidApiSessionError struct{}

func (InvalidApiSessionError) Error() string {
	return "invalid api session"
}

func (self InvalidApiSessionError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidApiSession
}

func (InvalidApiSessionError) GetRetryType() edge_ctrl_pb.CreateTerminatorResult {
	return edge_ctrl_pb.CreateTerminatorResult_FailedStartOver
}

type InvalidSessionError struct{}

func (InvalidSessionError) Error() string {
	return "invalid session"
}

func (self InvalidSessionError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidSession
}

func (InvalidSessionError) GetRetryType() edge_ctrl_pb.CreateTerminatorResult {
	return edge_ctrl_pb.CreateTerminatorResult_FailedStartOver
}

type WrongSessionTypeError struct{}

func (WrongSessionTypeError) Error() string {
	return "incorrect session type"
}

func (self WrongSessionTypeError) ErrorCode() uint32 {
	return edge.ErrorCodeWrongSessionType
}

func (WrongSessionTypeError) GetRetryType() edge_ctrl_pb.CreateTerminatorResult {
	return edge_ctrl_pb.CreateTerminatorResult_FailedStartOver
}

type InvalidEdgeRouterForSessionError struct{}

func (InvalidEdgeRouterForSessionError) Error() string {
	return "invalid edge router for session"
}

func (self InvalidEdgeRouterForSessionError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidEdgeRouterForSession
}

func (InvalidEdgeRouterForSessionError) GetRetryType() edge_ctrl_pb.CreateTerminatorResult {
	return edge_ctrl_pb.CreateTerminatorResult_FailedStartOver
}

type InvalidServiceError struct{}

func (InvalidServiceError) Error() string {
	return "invalid service"
}

func (self InvalidServiceError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidService
}

func (InvalidServiceError) GetRetryType() edge_ctrl_pb.CreateTerminatorResult {
	return edge_ctrl_pb.CreateTerminatorResult_FailedPermanent
}

type TunnelingNotEnabledError struct{}

func (TunnelingNotEnabledError) Error() string {
	return "tunneling not enabled"
}

func (self TunnelingNotEnabledError) ErrorCode() uint32 {
	return edge.ErrorCodeTunnelingNotEnabled
}

func (TunnelingNotEnabledError) GetRetryType() edge_ctrl_pb.CreateTerminatorResult {
	return edge_ctrl_pb.CreateTerminatorResult_FailedPermanent
}

func invalidTerminator(msg string) controllerError {
	return &genericControllerError{
		msg:       msg,
		errorCode: edge.ErrorCodeInvalidTerminator,
		retryType: edge_ctrl_pb.CreateTerminatorResult_FailedPermanent,
	}
}

func invalidCost(msg string) controllerError {
	return &genericControllerError{
		msg:       msg,
		errorCode: edge.ErrorCodeInvalidCost,
		retryType: edge_ctrl_pb.CreateTerminatorResult_FailedPermanent,
	}
}

func invalidPrecedence(msg string) controllerError {
	return &genericControllerError{
		msg:       msg,
		errorCode: edge.ErrorCodeInvalidPrecedence,
		retryType: edge_ctrl_pb.CreateTerminatorResult_FailedPermanent,
	}
}

func encryptionDataMissing(msg string) controllerError {
	return &genericControllerError{
		msg:       msg,
		errorCode: edge.ErrorCodeEncryptionDataMissing,
		retryType: edge_ctrl_pb.CreateTerminatorResult_FailedPermanent,
	}
}

type genericControllerError struct {
	msg       string
	errorCode uint32
	retryType edge_ctrl_pb.CreateTerminatorResult
}

func (self *genericControllerError) Error() string {
	return self.msg
}

func (self *genericControllerError) ErrorCode() uint32 {
	return self.errorCode
}

func (self *genericControllerError) GetRetryType() edge_ctrl_pb.CreateTerminatorResult {
	return self.retryType
}
