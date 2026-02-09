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

package apierror

import (
	"fmt"

	"github.com/openziti/foundation/v2/errorz"
)

func NewCouldNotParseBody(err error) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: CouldNotParseBodyCode,
		Message: CouldNotParseBodyMessage,
		Status:  CouldNotParseBodyStatus,
		Cause:   err,
	}
}

func NewInvalidContentType(contentType string) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidContentTypeCode,
		Message: InvalidContentTypeMessage + ": " + contentType,
		Status:  InvalidContentTypeStatus,
	}
}

func NewCouldNotReadBody(err error) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode:     CouldNotReadBodyCode,
		Message:     CouldNotReadBodyMessage,
		Status:      CouldNotReadBodyStatus,
		Cause:       err,
		AppendCause: false,
	}
}

func NewInvalidAuth() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidAuthCode,
		Message: InvalidAuthMessage,
		Status:  InvalidAuthStatus,
	}
}

func NewInvalidAuthMethod() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidAuthMethodCode,
		Message: InvalidAuthMethodMessage,
		Status:  InvalidAuthMethodStatus,
	}
}

func NewEnrollmentExpired() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: EnrollmentExpiredCode,
		Message: EnrollmentExpiredMessage,
		Status:  EnrollmentExpiredStatus,
	}
}

func NewEnrollmentIdentityAlreadyEnrolled() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: EnrollmentIdentityAlreadyEnrolledCode,
		Message: EnrollmentIdentityAlreadyEnrolledMessage,
		Status:  EnrollmentIdentityAlreadyEnrolledStatus,
	}
}

func NewCouldNotProcessCsr() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: CouldNotProcessCsrCode,
		Message: CouldNotProcessCsrMessage,
		Status:  CouldNotProcessCsrStatus,
	}
}

func NewEnrollmentCaNoLongValid() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: EnrollmentCaNoLongValidCode,
		Message: EnrollmentCaNoLongValidMessage,
		Status:  EnrollmentCaNoLongValidStatus,
	}
}

func NewEnrollmentNoValidCas() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: EnrollmentNoValidCasCode,
		Message: EnrollmentNoValidCasMessage,
		Status:  EnrollmentNoValidCasStatus,
	}
}

func NewInvalidEnrollmentToken() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidEnrollmentTokenCode,
		Message: InvalidEnrollmentTokenMessage,
		Status:  InvalidEnrollmentTokenStatus,
	}
}

func NewInvalidEnrollmentNotAllowed() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidEnrollmentNotAllowedCode,
		Message: InvalidEnrollmentNotAllowedMessage,
		Status:  InvalidEnrollmentNotAllowedStatus,
	}
}

func NewInvalidEnrollmentAlreadyEnrolled() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidEnrollmentAlreadyEnrolledCode,
		Message: InvalidEnrollmentAlreadyEnrolledMessage,
		Status:  InvalidEnrollmentAlreadyEnrolledStatus,
	}
}

func NewInvalidEnrollMethod() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidEnrollMethodCode,
		Message: InvalidEnrollMethodMessage,
		Status:  InvalidEnrollMethodStatus,
	}
}

func NewCouldNotParseX509FromDer() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: CouldNotParseX509FromDerCode,
		Message: CouldNotParseX509FromDerMessage,
		Status:  CouldNotParseX509FromDerStatus,
	}
}

func NewCertFailedValidation() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: CertFailedValidationCode,
		Message: CertFailedValidationMessage,
		Status:  CertFailedValidationStatus,
	}
}

func NewCertInUse() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: CertInUseCode,
		Message: CertInUseMessage,
		Status:  CertInUseStatus,
	}
}

func NewCaAlreadyVerified() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: CaAlreadyVerifiedCode,
		Message: CaAlreadyVerifiedMessage,
		Status:  CaAlreadyVerifiedStatus,
	}
}

func NewExpectedPemBlockCertificate() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: ExpectedPemBlockCertificateCode,
		Message: ExpectedPemBlockCertificateMessage,
		Status:  ExpectedPemBlockCertificateStatus,
	}
}

func NewCouldNotParseDerBlock() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: CouldNotParseDerBlockCode,
		Message: CouldNotParseDerBlockMessage,
		Status:  CouldNotParseDerBlockStatus,
	}
}

func NewCouldNotParsePem() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: CouldNotParsePemCode,
		Message: CouldNotParsePemMessage,
		Status:  CouldNotParsePemStatus,
	}
}

func NewInvalidCommonName() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidCommonNameCode,
		Message: InvalidCommonNameMessage,
		Status:  InvalidCommonNameStatus,
	}
}

func NewFailedCertificateValidation() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: FailedCertificateValidationCode,
		Message: FailedCertificateValidationMessage,
		Status:  FailedCertificateValidationStatus,
	}
}

func NewInvalidEnrollmentMissingCsr(cause error) *errorz.ApiError {
	return &errorz.ApiError{
		Cause:   cause,
		AppCode: InvalidEnrollmentMissingCsrCode,
		Message: InvalidEnrollmentMissingCsrMessage,
		Status:  InvalidEnrollmentMissingCsrStatus,
	}
}

func NewCertificateIsNotCa() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: CertificateIsNotCaCode,
		Message: CertificateIsNotCaMessage,
		Status:  CertificateIsNotCaStatus,
	}
}

func NewInvalidUuid(val string) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidUuidCode,
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

func NewInvalidAuthenticatorProperties() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidAuthenticatorPropertiesCode,
		Message: InvalidAuthenticatorPropertiesMessage,
		Status:  InvalidAuthenticatorPropertiesStatus,
	}
}

func NewAuthenticatorCannotBeUpdated() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: AuthenticatorCanNotBeUpdatedCode,
		Message: AuthenticatorCanNotBeUpdatedMessage,
		Status:  AuthenticatorCanNotBeUpdatedStatus,
	}
}

func NewFabricRouterCannotBeUpdate() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: RouterCanNotBeUpdatedCode,
		Message: RouterCanNotBeUpdatedMessage,
		Status:  RouterCanNotBeUpdatedStatus,
	}
}

func NewAuthenticatorMethodMax() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: AuthenticatorMethodMaxCode,
		Message: AuthenticatorMethodMaxMessage,
		Status:  AuthenticatorMethodMaxStatus,
	}
}

func NewMethodNotAllowed() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: MethodNotAllowedCode,
		Message: MethodNotAllowedMessage,
		Status:  MethodNotAllowedStatus,
	}
}

func NewRateLimited() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: RateLimitedCode,
		Message: RateLimitedMessage,
		Status:  RateLimitedStatus,
	}
}

func NewTimeoutError() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: TimeoutCode,
		Message: TimeoutMessage,
		Status:  TimeoutStatus,
	}
}

func NewNoEdgeRoutersAvailable() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: NoEdgeRoutersAvailableCode,
		Message: NoEdgeRoutersAvailableMessage,
		Status:  NoEdgeRoutersAvailableStatus,
	}
}

func NewMissingCertClaim() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: MissingCertClaimCode,
		Message: MissingCertClaimMessage,
		Status:  MissingCertClaimStatus,
	}
}

func NewInvalidPosture(cause error) *errorz.ApiError {
	return &errorz.ApiError{
		Cause:   cause,
		AppCode: InvalidPostureCode,
		Message: InvalidPostureMessage,
		Status:  InvalidPostureStatus,
	}
}

func NewMfaExistsError() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: MfaExistsCode,
		Message: MfaExistsMessage,
		Status:  MfaExistsStatus,
	}
}

func NewMfaEnrollmentNotStarted() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: MfaEnrollmentNotStartedCode,
		Message: MfaEnrollmentNotStartedMessage,
		Status:  MfaEnrollmentNotStartedStatus,
	}
}

func NewMfaNotEnrolledError() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: MfaNotEnrolledCode,
		Message: MfaNotEnrolledMessage,
		Status:  MfaNotEnrolledStatus,
	}
}

func NewInvalidMfaTokenError() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: MfaInvalidTokenCode,
		Message: MfaInvalidTokenMessage,
		Status:  MfaInvalidTokenStatus,
	}
}

func NewInvalidBackingTokenTypeError() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidBackingTokenTypeCode,
		Message: InvalidBackingTokenTypeMessage,
		Status:  InvalidBackingTokenTypeStatus,
	}
}

func NewEdgeRouterFailedReEnrollment(cause error) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode:     EdgeRouterFailedReEnrollmentCode,
		Message:     EdgeRouterFailedReEnrollmentMessage,
		Status:      EdgeRouterFailedReEnrollmentStatus,
		Cause:       cause,
		AppendCause: true,
	}
}

func NewInvalidClientCertificate() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidClientCertCode,
		Message: InvalidClientCertMessage,
		Status:  InvalidClientCertStatus,
	}
}

func NewInvalidCertificatePem() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: InvalidCertificatePemCode,
		Message: InvalidCertificatePemMessage,
		Status:  InvalidCertificatePemStatus,
	}
}

func NewCanNotDeleteReferencedEntity(localEntityType, remoteEntityType string, referencingEntityTypeIds []string, fieldName string) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode:     CanNotDeleteReferencedEntityCode,
		Message:     CanNotDeleteReferencedEntityMessage,
		Status:      CanNotDeleteReferencedEntityStatus,
		Cause:       errorz.NewFieldError(fmt.Sprintf("entity type %s referenced by %s: %v", localEntityType, remoteEntityType, referencingEntityTypeIds), fieldName, referencingEntityTypeIds),
		AppendCause: true,
	}
}

func NewBadRequestFieldError(fieldError errorz.FieldError) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode:     errorz.InvalidFieldCode,
		Message:     errorz.InvalidFieldMessage,
		Status:      errorz.InvalidFieldStatus,
		Cause:       fieldError,
		AppendCause: true,
	}
}

func NewEnrollmentExists(enrollmentMethod string) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode:     EnrollmentExistsCode,
		Message:     EnrollmentExistsMessage,
		Status:      EnrollmentExistsStatus,
		Cause:       errorz.NewFieldError("enrollment of same method exists", "method", enrollmentMethod),
		AppendCause: true,
	}
}

func NewTooManyUpdatesError() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: ServerTooManyRequestsCode,
		Message: ServerTooManyRequestsMessage,
		Status:  ServerTooManyRequestsStatus,
	}
}

func NewNotRunningInHAModeError() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: ServerNotRunningInHAModeCode,
		Message: ServerNotRunningInHAModeMessage,
		Status:  ServerNotRunningInHAModeStatus,
	}
}

func NewClusterHasNoLeaderError() *errorz.ApiError {
	return &errorz.ApiError{
		AppCode: ClusterHasNoLeaderCode,
		Message: ClusterHasNoLeaderMessage,
		Status:  ClusterHasNoLeaderStatus,
	}
}

func NewTransferLeadershipError(err error) *errorz.ApiError {
	return &errorz.ApiError{
		AppCode:     TransferLeadershipErrorCode,
		Message:     TransferLeadershipErrorMessage,
		Status:      TransferLeadershipErrorStatus,
		Cause:       err,
		AppendCause: true,
	}
}
