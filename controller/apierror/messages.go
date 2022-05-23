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

import "net/http"

const (
	MethodNotAllowedCode    string = "METHOD_NOT_ALLOWED"
	MethodNotAllowedMessage string = "The resource requested does not support this HTTP verb"
	MethodNotAllowedStatus  int    = http.StatusMethodNotAllowed

	CouldNotParseBodyCode    string = "COULD_NOT_PARSE_BODY"
	CouldNotParseBodyMessage string = "The body of the request could not be parsed"
	CouldNotParseBodyStatus  int    = http.StatusBadRequest

	CouldNotReadBodyCode    string = "COULD_NOT_READ_BODY"
	CouldNotReadBodyMessage string = "The body of the request could not be read"
	CouldNotReadBodyStatus  int    = http.StatusInternalServerError

	InvalidUuidCode    string = "INVALID_UUID"
	InvalidUuidMessage string = "The supplied UUID is invalid"
	InvalidUuidStatus  int    = http.StatusBadRequest

	InvalidContentTypeCode    string = "INVALID_CONTENT_TYPE"
	InvalidContentTypeMessage string = "The content type supplied is not acceptable"
	InvalidContentTypeStatus  int    = http.StatusBadRequest
)

// specific
const (
	InvalidAuthCode    string = "INVALID_AUTH"
	InvalidAuthMessage string = "The authentication request failed"
	InvalidAuthStatus  int    = http.StatusUnauthorized

	InvalidAuthMethodCode    string = "INVALID_AUTH_METHOD"
	InvalidAuthMethodMessage string = "The supplied authentication method is not valid"
	InvalidAuthMethodStatus  int    = http.StatusBadRequest

	EnrollmentExpiredCode    string = "ENROLLMENT_EXPIRED"
	EnrollmentExpiredMessage string = "The window for this enrollment has expired"
	EnrollmentExpiredStatus  int    = http.StatusBadRequest

	CouldNotProcessCsrCode    string = "COULD_NOT_PROCESS_CSR"
	CouldNotProcessCsrMessage string = "The supplied csr could not be processed"
	CouldNotProcessCsrStatus  int    = http.StatusBadRequest

	EnrollmentCaNoLongValidCode    string = "ENROLLMENT_CA_NOT_VALID"
	EnrollmentCaNoLongValidMessage string = "The CA tied to the supplied token is no longer valid"
	EnrollmentCaNoLongValidStatus  int    = http.StatusBadRequest

	EnrollmentNoValidCasCode    string = "ENROLLMENT_NO_VALID_CAS"
	EnrollmentNoValidCasMessage string = "No CAs are valid for this request"
	EnrollmentNoValidCasStatus  int    = http.StatusBadRequest

	InvalidEnrollmentTokenCode    string = "INVALID_ENROLLMENT_TOKEN"
	InvalidEnrollmentTokenMessage string = "The supplied token is not valid"
	InvalidEnrollmentTokenStatus  int    = http.StatusBadRequest

	InvalidEnrollMethodCode    string = "INVALID_ENROLL_METHOD"
	InvalidEnrollMethodMessage string = "The supplied enrollment method is not valid"
	InvalidEnrollMethodStatus  int    = http.StatusBadRequest

	InvalidEnrollmentMissingCsrCode    string = "MISSING_OR_INVALID_CSR"
	InvalidEnrollmentMissingCsrMessage string = "The supplied enrollment request is missing or contains an invalid CSR"
	InvalidEnrollmentMissingCsrStatus  int    = http.StatusBadRequest

	NoEdgeRoutersAvailableCode    string = "NO_EDGE_ROUTERS_AVAILABLE"
	NoEdgeRoutersAvailableMessage string = "No edge routers are assigned and online to handle the requested connection"
	NoEdgeRoutersAvailableStatus  int    = http.StatusBadRequest

	CouldNotDecodeProxiedCertCode    string = "COULD_NOT_PARSE_PROXY_CERT"
	CouldNotDecodeProxiedCertMessage string = "could not decode proxy client cert"
	CouldNotDecodeProxiedCertStatus  int    = http.StatusInternalServerError

	CouldNotParseX509FromDerCode    string = "COULD_NOT_PARSE_x509_FROM_DER"
	CouldNotParseX509FromDerMessage string = "could not parse x509 from DER"
	CouldNotParseX509FromDerStatus  int    = http.StatusBadRequest

	CertFailedValidationCode    string = "CERT_FAILED_VALIDATION"
	CertFailedValidationMessage string = "certificate failed to validate against CA"
	CertFailedValidationStatus  int    = http.StatusBadRequest

	CertInUseCode    string = "CERT_IN_USE"
	CertInUseMessage string = "The certificate supplied is already associated with another identity"
	CertInUseStatus  int    = http.StatusConflict

	MissingCertClaimCode    string = "MISSING_CERT_CLAIM"
	MissingCertClaimMessage string = "The certificate is expected to contain and externalId, which was not found"
	MissingCertClaimStatus  int    = http.StatusBadRequest

	CaAlreadyVerifiedCode    string = "CA_ALREADY_VERIFIED"
	CaAlreadyVerifiedMessage string = "CA has already been verified"
	CaAlreadyVerifiedStatus  int    = http.StatusConflict

	ExpectedPemBlockCertificateCode           = "EXPECTED_PEM_CERTIFICATE"
	ExpectedPemBlockCertificateMessage string = "expected PEM block type 'CERTIFICATE'"
	ExpectedPemBlockCertificateStatus  int    = http.StatusBadRequest

	CouldNotParseDerBlockCode    string = "COULD_NOT_PARSE_DER_BLOCK"
	CouldNotParseDerBlockMessage string = "The certificate's DER block could not be parsed"
	CouldNotParseDerBlockStatus  int    = http.StatusBadRequest

	CouldNotParsePemCode    string = "COULD_NOT_PARSE_PEM_BLOCK"
	CouldNotParsePemMessage string = "The certificate's PEM block could not be parsed"
	CouldNotParsePemStatus  int    = http.StatusBadRequest

	InvalidCommonNameCode    string = "INVALID_COMMON_NAME"
	InvalidCommonNameMessage string = "The common name of the supplied certificate is invalid"
	InvalidCommonNameStatus  int    = http.StatusBadRequest

	FailedCertificateValidationCode    string = "CERTIFICATE_FAILED_VALIDATION"
	FailedCertificateValidationMessage string = "The supplied certificate failed to validate against the CA's certificate chain"
	FailedCertificateValidationStatus  int    = http.StatusBadRequest

	CertificateIsNotCaCode    string = "CERTIFICATE_IS_NOT_CA"
	CertificateIsNotCaMessage string = "Leading certificate is not a CA"
	CertificateIsNotCaStatus  int    = http.StatusBadRequest

	InvalidAuthenticatorPropertiesCode    string = "INVALID_AUTHENTICATOR_PROPERTIES"
	InvalidAuthenticatorPropertiesMessage string = "The properties supplied did not match the authenticator method"
	InvalidAuthenticatorPropertiesStatus  int    = http.StatusBadRequest

	AuthenticatorCanNotBeUpdatedCode    string = "CAN_NOT_UPDATE_AUTHENTICATOR"
	AuthenticatorCanNotBeUpdatedMessage string = "The authenticator cannot be updated in this fashion"
	AuthenticatorCanNotBeUpdatedStatus  int    = http.StatusConflict

	RouterCanNotBeUpdatedCode    string = "CAN_NOT_UPDATE_ROUTER"
	RouterCanNotBeUpdatedMessage string = "The router was not added via the Edge API and cannot be updated"
	RouterCanNotBeUpdatedStatus  int    = http.StatusConflict

	AuthenticatorMethodMaxCode    string = "MAX_AUTHENTICATOR_METHODS_REACHED"
	AuthenticatorMethodMaxMessage string = "The identity already has the maximum authenticators of the specified method"
	AuthenticatorMethodMaxStatus  int    = http.StatusConflict

	RateLimitedCode    string = "RATE_LIMITED"
	RateLimitedMessage string = "The requested operation is rate limited and the rate limit has been exceeded. Please try again later"
	RateLimitedStatus  int    = http.StatusTooManyRequests

	TimeoutCode    string = "TIMEOUT"
	TimeoutMessage string = "The requested operation took too much time to reply"
	TimeoutStatus  int    = http.StatusServiceUnavailable

	InvalidPostureCode    string = "INVALID_POSTURE"
	InvalidPostureMessage string = "Posture response data is missing or wrong"
	InvalidPostureStatus  int    = http.StatusConflict

	MfaExistsCode    string = "MFA_EXISTS"
	MfaExistsMessage string = "An MFA record already exists, try removing it"
	MfaExistsStatus  int    = http.StatusConflict

	/* #nosec */
	MfaInvalidTokenCode    string = "MFA_INVALID_TOKEN"
	MfaInvalidTokenMessage string = "An invalid token/code was provided"
	MfaInvalidTokenStatus  int    = http.StatusBadRequest

	MfaNotEnrolledCode    string = "MFA_NOT_ENROLLED"
	MfaNotEnrolledMessage string = "The current identity is not enrolled in MFA"
	MfaNotEnrolledStatus  int    = http.StatusConflict

	EdgeRouterFailedReEnrollmentCode        = "FAILED_ER_REENROLLMENT"
	EdgeRouterFailedReEnrollmentMessage     = "the edge router failed to be re-enrolled, see cause"
	EdgeRouterFailedReEnrollmentStatus  int = http.StatusInternalServerError

	InvalidClientCertCode    string = "INVALID_CLIENT_CERT"
	InvalidClientCertMessage string = "The provided client certificate is invalid"
	InvalidClientCertStatus  int    = http.StatusBadRequest

	InvalidCertificatePemCode    string = "INVALID_CERT_PEM"
	InvalidCertificatePemMessage string = "the supplied certificate PEM is either invalid or contains the incorrect number of certificates"
	InvalidCertificatePemStatus  int    = http.StatusBadRequest

	CanNotDeleteReferencedEntityCode    string = "CAN_NOT_DELETE_REFERENCED_ENTITY"
	CanNotDeleteReferencedEntityMessage string = "the entity cannot be deleted because it is referenced by another entity, see cause"
	CanNotDeleteReferencedEntityStatus  int    = http.StatusConflict

	ReferencedEntityNotFoundCode    string = "REFERENCED_ENTITY_NOT_FOUND"
	ReferencedEntityNotFoundMessage string = "REFERENCED_ENTITY_NOT_FOUND"
	ReferencedEntityNotFoundStatus  int    = http.StatusBadRequest

	EnrollmentExistsCode    string = "ENROLLMENT_EXISTS"
	EnrollmentExistsMessage string = "ENROLLMENT_EXISTS"
	EnrollmentExistsStatus  int    = http.StatusConflict
)
