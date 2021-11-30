/*
	Copyright NetFoundry, Inc.

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

package apierror

import (
	"github.com/openziti/fabric/controller/apierror"
	"github.com/openziti/foundation/util/errorz"
)

func NewCouldNotParseBody(err error) *errorz.ApiError {
	return &errorz.ApiError{
		Code:    CouldNotParseBodyCode,
		Message: CouldNotParseBodyMessage,
		Status:  CouldNotParseBodyStatus,
		Cause:   err,
	}
}

func NewInvalidContentType(contentType string) *errorz.ApiError {
	return &errorz.ApiError{
		Code:    InvalidContentTypeCode,
		Message: InvalidContentTypeMessage + ": " + contentType,
		Status:  InvalidContentTypeStatus,
	}
}

func NewCouldNotReadBody(err error) *errorz.ApiError {
	return &errorz.ApiError{
		Code:        CouldNotReadBodyCode,
		Message:     CouldNotReadBodyMessage,
		Status:      CouldNotReadBodyStatus,
		Cause:       err,
		AppendCause: false,
	}
}

func NewInvalidAuth() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    InvalidAuthCode,
		Message: InvalidAuthMessage,
		Status:  InvalidAuthStatus,
	}
}

func NewInvalidAuthMethod() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    InvalidAuthMethodCode,
		Message: InvalidAuthMethodMessage,
		Status:  InvalidAuthMethodStatus,
	}
}

func NewEnrollmentExpired() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    EnrollmentExpiredCode,
		Message: EnrollmentExpiredMessage,
		Status:  EnrollmentExpiredStatus,
	}
}

func NewCouldNotProcessCsr() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    CouldNotProcessCsrCode,
		Message: CouldNotProcessCsrMessage,
		Status:  CouldNotProcessCsrStatus,
	}
}

func NewEnrollmentCaNoLongValid() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    EnrollmentCaNoLongValidCode,
		Message: EnrollmentCaNoLongValidMessage,
		Status:  EnrollmentCaNoLongValidStatus,
	}
}

func NewEnrollmentNoValidCas() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    EnrollmentNoValidCasCode,
		Message: EnrollmentNoValidCasMessage,
		Status:  EnrollmentNoValidCasStatus,
	}
}

func NewInvalidEnrollmentToken() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    InvalidEnrollmentTokenCode,
		Message: InvalidEnrollmentTokenMessage,
		Status:  InvalidEnrollmentTokenStatus,
	}
}

func NewInvalidEnrollMethod() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    InvalidEnrollMethodCode,
		Message: InvalidEnrollMethodMessage,
		Status:  InvalidEnrollMethodStatus,
	}
}

func NewNoEdgeRoutersAvailable() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    NoEdgeRoutersAvailableCode,
		Message: NoEdgeRoutersAvailableMessage,
		Status:  NoEdgeRoutersAvailableStatus,
	}
}

func NewCouldNotParseX509FromDer() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    CouldNotParseX509FromDerCode,
		Message: CouldNotParseX509FromDerMessage,
		Status:  CouldNotParseX509FromDerStatus,
	}
}

func NewCertFailedValidation() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    CertFailedValidationCode,
		Message: CertFailedValidationMessage,
		Status:  CertFailedValidationStatus,
	}
}

func NewCertInUse() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    CertInUseCode,
		Message: CertInUseMessage,
		Status:  CertInUseStatus,
	}
}

func NewCaAlreadyVerified() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    CaAlreadyVerifiedCode,
		Message: CaAlreadyVerifiedMessage,
		Status:  CaAlreadyVerifiedStatus,
	}
}

func NewExpectedPemBlockCertificate() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    ExpectedPemBlockCertificateCode,
		Message: ExpectedPemBlockCertificateMessage,
		Status:  ExpectedPemBlockCertificateStatus,
	}
}

func NewCouldNotParseDerBlock() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    CouldNotParseDerBlockCode,
		Message: CouldNotParseDerBlockMessage,
		Status:  CouldNotParseDerBlockStatus,
	}
}

func NewCouldNotParsePem() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    CouldNotParsePemCode,
		Message: CouldNotParsePemMessage,
		Status:  CouldNotParsePemStatus,
	}
}

func NewInvalidCommonName() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    InvalidCommonNameCode,
		Message: InvalidCommonNameMessage,
		Status:  InvalidCommonNameStatus,
	}
}

func NewFailedCertificateValidation() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    FailedCertificateValidationCode,
		Message: FailedCertificateValidationMessage,
		Status:  FailedCertificateValidationStatus,
	}
}

func NewInvalidEnrollmentMissingCsr(cause error) *errorz.ApiError {
	return &errorz.ApiError{
		Cause:   cause,
		Code:    InvalidEnrollmentMissingCsrCode,
		Message: InvalidEnrollmentMissingCsrMessage,
		Status:  InvalidEnrollmentMissingCsrStatus,
	}
}

func NewCertificateIsNotCa() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    CertificateIsNotCaCode,
		Message: CertificateIsNotCaMessage,
		Status:  CertificateIsNotCaStatus,
	}
}

func NewInvalidUuid(val string) *errorz.ApiError {
	return &errorz.ApiError{
		Code:    InvalidUuidCode,
		Message: InvalidUuidMessage,
		Status:  InvalidUuidStatus,
		Cause: &apierror.GenericCauseError{
			Message: "invalid uuid",
			DataMap: map[string]interface{}{
				"uuid": val,
			},
		},
	}
}

func NewInvalidAuthenticatorProperties() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    InvalidAuthenticatorPropertiesCode,
		Message: InvalidAuthenticatorPropertiesMessage,
		Status:  InvalidAuthenticatorPropertiesStatus,
	}
}

func NewAuthenticatorCannotBeUpdated() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    AuthenticatorCanNotBeUpdatedCode,
		Message: AuthenticatorCanNotBeUpdatedMessage,
		Status:  AuthenticatorCanNotBeUpdatedStatus,
	}
}

func NewFabricRouterCannotBeUpdate() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    RouterCanNotBeUpdatedCode,
		Message: RouterCanNotBeUpdatedMessage,
		Status:  RouterCanNotBeUpdatedStatus,
	}
}

func NewAuthenticatorMethodMax() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    AuthenticatorMethodMaxCode,
		Message: AuthenticatorMethodMaxMessage,
		Status:  AuthenticatorMethodMaxStatus,
	}
}

func NewMethodNotAllowed() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    MethodNotAllowedCode,
		Message: MethodNotAllowedMessage,
		Status:  MethodNotAllowedStatus,
	}
}

func NewRateLimited() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    RateLimitedCode,
		Message: RateLimitedMessage,
		Status:  RateLimitedStatus,
	}
}

func NewTimeoutError() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    TimeoutCode,
		Message: TimeoutMessage,
		Status:  TimeoutStatus,
	}
}

func NewInvalidPosture(cause error) *errorz.ApiError {
	return &errorz.ApiError{
		Cause:   cause,
		Code:    InvalidPostureCode,
		Message: InvalidPostureMessage,
		Status:  InvalidPostureStatus,
	}
}

func NewMfaExistsError() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    MfaExistsCode,
		Message: MfaExistsMessage,
		Status:  MfaExistsStatus,
	}
}

func NewMfaNotEnrolledError() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    MfaNotEnrolledCode,
		Message: MfaNotEnrolledMessage,
		Status:  MfaNotEnrolledStatus,
	}
}

func NewInvalidMfaTokenError() *errorz.ApiError {
	return &errorz.ApiError{
		Code:    MfaInvalidTokenCode,
		Message: MfaInvalidTokenMessage,
		Status:  MfaInvalidTokenStatus,
	}
}
