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
	"net/http"
)

func NewNotFound() *ApiError {
	return &ApiError{
		Code:    NotFoundCode,
		Message: NotFoundMessage,
		Status:  NotFoundStatus,
	}
}

func NewUnhandled(cause error) *ApiError {
	return &ApiError{
		Code:    UnhandledCode,
		Message: UnhandledMessage,
		Status:  UnhandledStatus,
		Cause:   cause,
	}
}
func NewCouldNotParseBody(err error) *ApiError {
	return &ApiError{
		Code:    CouldNotParseBodyCode,
		Message: CouldNotParseBodyMessage,
		Status:  CouldNotParseBodyStatus,
		Cause:   err,
	}
}

func NewInvalidContentType(contentType string) *ApiError {
	return &ApiError{
		Code:    InvalidContentTypeCode,
		Message: InvalidContentTypeMessage + ": " + contentType,
		Status:  InvalidContentTypeStatus,
	}
}

func NewCouldNotReadBody(err error) *ApiError {
	return &ApiError{
		Code:        CouldNotReadBodyCode,
		Message:     CouldNotReadBodyMessage,
		Status:      CouldNotReadBodyStatus,
		Cause:       err,
		AppendCause: false,
	}
}
func NewInvalidField() *ApiError {
	return &ApiError{
		Code:    InvalidFieldCode,
		Message: InvalidFieldMessage,
		Status:  InvalidFieldStatus,
	}
}

func NewEntityCanNotBeDeleted() *ApiError {
	return &ApiError{
		Code:    EntityCanNotBeDeletedCode,
		Message: EntityCanNotBeDeletedMessage,
		Status:  EntityCanNotBeDeletedStatus,
	}
}
func NewInvalidAuth() *ApiError {
	return &ApiError{
		Code:    InvalidAuthCode,
		Message: InvalidAuthMessage,
		Status:  InvalidAuthStatus,
	}
}
func NewInvalidAuthMethod() *ApiError {
	return &ApiError{
		Code:    InvalidAuthMethodCode,
		Message: InvalidAuthMethodMessage,
		Status:  InvalidAuthMethodStatus,
	}
}
func NewEnrollmentExpired() *ApiError {
	return &ApiError{
		Code:    EnrollmentExpiredCode,
		Message: EnrollmentExpiredMessage,
		Status:  EnrollmentExpiredStatus,
	}
}
func NewCouldNotProcessCsr() *ApiError {
	return &ApiError{
		Code:    CouldNotProcessCsrCode,
		Message: CouldNotProcessCsrMessage,
		Status:  CouldNotProcessCsrStatus,
	}
}
func NewEnrollmentCaNoLongValid() *ApiError {
	return &ApiError{
		Code:    EnrollmentCaNoLongValidCode,
		Message: EnrollmentCaNoLongValidMessage,
		Status:  EnrollmentCaNoLongValidStatus,
	}
}
func NewEnrollmentNoValidCas() *ApiError {
	return &ApiError{
		Code:    EnrollmentNoValidCasCode,
		Message: EnrollmentNoValidCasMessage,
		Status:  EnrollmentNoValidCasStatus,
	}
}
func NewInvalidEnrollmentToken() *ApiError {
	return &ApiError{
		Code:    InvalidEnrollmentTokenCode,
		Message: InvalidEnrollmentTokenMessage,
		Status:  InvalidEnrollmentTokenStatus,
	}
}
func NewInvalidEnrollMethod() *ApiError {
	return &ApiError{
		Code:    InvalidEnrollMethodCode,
		Message: InvalidEnrollMethodMessage,
		Status:  InvalidEnrollMethodStatus,
	}
}
func NewInvalidFilter(cause error) *ApiError {
	return &ApiError{
		Code:        InvalidFilterCode,
		Message:     InvalidFilterMessage,
		Status:      InvalidFilterStatus,
		Cause:       cause,
		AppendCause: true,
	}
}
func NewInvalidPagination() *ApiError {
	return &ApiError{
		Code:    InvalidPaginationCode,
		Message: InvalidPaginationMessage,
		Status:  InvalidPaginationStatus,
	}
}
func NewNoEdgeRoutersAvailable() *ApiError {
	return &ApiError{
		Code:    NoEdgeRoutersAvailableCode,
		Message: NoEdgeRoutersAvailableMessage,
		Status:  NoEdgeRoutersAvailableStatus,
	}
}
func NewInvalidSort() *ApiError {
	return &ApiError{
		Code:    InvalidSortCode,
		Message: InvalidSortMessage,
		Status:  InvalidSortStatus,
	}
}
func NewCouldNotValidate(err error) *ApiError {
	return &ApiError{
		Code:    CouldNotValidateCode,
		Message: CouldNotValidateMessage,
		Status:  CouldNotValidateStatus,
		Cause:   err,
	}
}
func NewUnauthorized() *ApiError {
	return &ApiError{
		Code:    UnauthorizedCode,
		Message: UnauthorizedMessage,
		Status:  UnauthorizedStatus,
	}
}
func NewCouldNotParseX509FromDer() *ApiError {
	return &ApiError{
		Code:    CouldNotParseX509FromDerCode,
		Message: CouldNotParseX509FromDerMessage,
		Status:  CouldNotParseX509FromDerStatus,
	}
}
func NewCertFailedValidation() *ApiError {
	return &ApiError{
		Code:    CertFailedValidationCode,
		Message: CertFailedValidationMessage,
		Status:  CertFailedValidationStatus,
	}
}
func NewCertInUse() *ApiError {
	return &ApiError{
		Code:    CertInUseCode,
		Message: CertInUseMessage,
		Status:  CertInUseStatus,
	}
}
func NewCaAlreadyVerified() *ApiError {
	return &ApiError{
		Code:    CaAlreadyVerifiedCode,
		Message: CaAlreadyVerifiedMessage,
		Status:  CaAlreadyVerifiedStatus,
	}
}
func NewExpectedPemBlockCertificate() *ApiError {
	return &ApiError{
		Code:    ExpectedPemBlockCertificateCode,
		Message: ExpectedPemBlockCertificateMessage,
		Status:  ExpectedPemBlockCertificateStatus,
	}
}
func NewCouldNotParseDerBlock() *ApiError {
	return &ApiError{
		Code:    CouldNotParseDerBlockCode,
		Message: CouldNotParseDerBlockMessage,
		Status:  CouldNotParseDerBlockStatus,
	}
}
func NewCouldNotParsePem() *ApiError {
	return &ApiError{
		Code:    CouldNotParsePemCode,
		Message: CouldNotParsePemMessage,
		Status:  CouldNotParsePemStatus,
	}
}
func NewInvalidCommonName() *ApiError {
	return &ApiError{
		Code:    InvalidCommonNameCode,
		Message: InvalidCommonNameMessage,
		Status:  InvalidCommonNameStatus,
	}
}
func NewFailedCertificateValidation() *ApiError {
	return &ApiError{
		Code:    FailedCertificateValidationCode,
		Message: FailedCertificateValidationMessage,
		Status:  FailedCertificateValidationStatus,
	}
}
func NewCertificateIsNotCa() *ApiError {
	return &ApiError{
		Code:    CertificateIsNotCaCode,
		Message: CertificateIsNotCaMessage,
		Status:  CertificateIsNotCaStatus,
	}
}

func NewField(fieldError *FieldError) *ApiError {
	return &ApiError{
		Code:        InvalidFieldCode,
		Message:     InvalidFieldMessage,
		Status:      http.StatusBadRequest,
		Cause:       fieldError,
		AppendCause: true,
	}
}

func NewInvalidUuid(val string) *ApiError {
	return &ApiError{
		Code:    InvalidUuidCode,
		Message: InvalidUuidMessage,
		Status:  InvalidUuidStatus,
		Cause: &GenericCauseError{
			Message: "invalid uuid",
			DataMap: map[string]interface{}{
				"uuid": val,
			},
		},
	}
}

func NewInvalidAuthenticatorProperties() *ApiError {
	return &ApiError{
		Code:    InvalidAuthenticatorPropertiesCode,
		Message: InvalidAuthenticatorPropertiesMessage,
		Status:  InvalidAuthenticatorPropertiesStatus,
	}
}

func NewAuthenticatorCannotBeUpdated() *ApiError {
	return &ApiError{
		Code:    AuthenticatorCanNotBeUpdatedCode,
		Message: AuthenticatorCanNotBeUpdatedMessage,
		Status:  AuthenticatorCanNotBeUpdatedStatus,
	}
}

func NewFabricRouterCannotBeUpdate() *ApiError {
	return &ApiError{
		Code:    RouterCanNotBeUpdatedCode,
		Message: RouterCanNotBeUpdatedMessage,
		Status:  RouterCanNotBeUpdatedStatus,
	}
}

func NewAuthenticatorMethodMax() *ApiError {
	return &ApiError{
		Code:    AuthenticatorMethodMaxCode,
		Message: AuthenticatorMethodMaxMessage,
		Status:  AuthenticatorMethodMaxStatus,
	}
}

func NewMethodNotAllowed() *ApiError {
	return &ApiError{
		Code:    MethodNotAllowedCode,
		Message: MethodNotAllowedMessage,
		Status:  MethodNotAllowedStatus,
	}
}

func NewRateLimited() *ApiError {
	return &ApiError{
		Code:    RateLimitedCode,
		Message: RateLimitedMessage,
		Status:  RateLimitedStatus,
	}
}

func NewTimeoutError() *ApiError {
	return &ApiError{
		Code:    TimeoutCode,
		Message: TimeoutMessage,
		Status:  TimeoutStatus,
	}
}