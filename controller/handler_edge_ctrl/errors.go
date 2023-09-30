package handler_edge_ctrl

import "github.com/openziti/sdk-golang/ziti/edge"

type controllerError interface {
	error
	ErrorCode() uint32
}

func internalError(err error) controllerError {
	if err == nil {
		return nil
	}
	return internalErrorWrapper{error: err}
}

type internalErrorWrapper struct {
	error
}

func (internalErrorWrapper) ErrorCode() uint32 {
	return edge.ErrorCodeInternal
}

type InvalidApiSessionError struct{}

func (InvalidApiSessionError) Error() string {
	return "invalid api session"
}

func (self InvalidApiSessionError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidApiSession
}

type InvalidSessionError struct{}

func (InvalidSessionError) Error() string {
	return "invalid session"
}

func (self InvalidSessionError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidSession
}

type WrongSessionTypeError struct{}

func (WrongSessionTypeError) Error() string {
	return "incorrect session type"
}

func (self WrongSessionTypeError) ErrorCode() uint32 {
	return edge.ErrorCodeWrongSessionType
}

type InvalidEdgeRouterForSessionError struct{}

func (InvalidEdgeRouterForSessionError) Error() string {
	return "invalid edge router for session"
}

func (self InvalidEdgeRouterForSessionError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidEdgeRouterForSession
}

type InvalidServiceError struct{}

func (InvalidServiceError) Error() string {
	return "invalid service"
}

func (self InvalidServiceError) ErrorCode() uint32 {
	return edge.ErrorCodeInvalidService
}

type TunnelingNotEnabledError struct{}

func (TunnelingNotEnabledError) Error() string {
	return "tunneling not enabled"
}

func (self TunnelingNotEnabledError) ErrorCode() uint32 {
	return edge.ErrorCodeTunnelingNotEnabled
}

func invalidTerminator(msg string) controllerError {
	return &genericControllerError{
		msg:       msg,
		errorCode: edge.ErrorCodeInvalidTerminator,
	}
}

func invalidCost(msg string) controllerError {
	return &genericControllerError{
		msg:       msg,
		errorCode: edge.ErrorCodeInvalidCost,
	}
}

func invalidPrecedence(msg string) controllerError {
	return &genericControllerError{
		msg:       msg,
		errorCode: edge.ErrorCodeInvalidPrecedence,
	}
}

func encryptionDataMissing(msg string) controllerError {
	return &genericControllerError{
		msg:       msg,
		errorCode: edge.ErrorCodeEncryptionDataMissing,
	}
}

type genericControllerError struct {
	msg       string
	errorCode uint32
}

func (self *genericControllerError) Error() string {
	return self.msg
}

func (self *genericControllerError) ErrorCode() uint32 {
	return self.errorCode
}
