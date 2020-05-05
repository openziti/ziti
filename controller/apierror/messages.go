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
	NotFoundCode    string = "NOT_FOUND"
	NotFoundMessage string = "The resource requested was not found or is no longer available"
	NotFoundStatus  int    = http.StatusNotFound

	MethodNotAllowedCode    string = "METHOD_NOT_ALLOWED"
	MethodNotAllowedMessage string = "The resource requested does not support this HTTP verb"
	MethodNotAllowedStatus  int    = http.StatusMethodNotAllowed

	UnhandledCode    string = "UNHANDLED"
	UnhandledMessage string = "An unhandled error occurred"
	UnhandledStatus  int    = http.StatusInternalServerError

	CouldNotParseBodyCode    string = "COULD_NOT_PARSE_BODY"
	CouldNotParseBodyMessage string = "The body of the request could not be parsed"
	CouldNotParseBodyStatus  int    = http.StatusBadRequest

	CouldNotReadBodyCode    string = "COULD_NOT_READ_BODY"
	CouldNotReadBodyMessage string = "The body of the request could not be read"
	CouldNotReadBodyStatus  int    = http.StatusInternalServerError

	InvalidFieldCode    string = "INVALID_FIELD"
	InvalidFieldMessage string = "The field contains an invalid value"
	InvalidFieldStatus  int    = http.StatusBadRequest

	EntityCanNotBeDeletedCode    string = "ENTITY_CAN_NOT_BE_DELETED"
	EntityCanNotBeDeletedMessage string = "The entity requested for delete can not be deleted"
	EntityCanNotBeDeletedStatus         = http.StatusBadRequest

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

	StatusBadFilter                = 480
	InvalidFilterCode       string = "INVALID_FILTER"
	InvalidFilterMessage    string = "The filter query supplied is invalid"
	httpStatusInvalidFilter        = StatusBadFilter
	InvalidFilterStatus     int    = httpStatusInvalidFilter

	InvalidPaginationCode    string = "INVALID_PAGINATION"
	InvalidPaginationMessage string = "The pagination properties provided are invalid"
	InvalidPaginationStatus  int    = http.StatusBadRequest

	NoEdgeRoutersAvailableCode    string = "NO_EDGE_ROUTERS_AVAILABLE"
	NoEdgeRoutersAvailableMessage string = "No edge routers are assigned and online to handle the requested connection"
	NoEdgeRoutersAvailableStatus  int    = http.StatusBadRequest

	InvalidSortCode    string = "INVALID_SORT_IDENTIFIER"
	InvalidSortMessage string = "The sort order supplied is invalid"
	InvalidSortStatus  int    = http.StatusBadRequest

	CouldNotValidateCode    string = "COULD_NOT_VALIDATE"
	CouldNotValidateMessage string = "The supplied request contains an invalid document"
	CouldNotValidateStatus  int    = http.StatusBadRequest

	UnauthorizedCode    string = "UNAUTHORIZED"
	UnauthorizedMessage string = "The request could not be completed. The session is not authorized or the credentials are invalid"
	UnauthorizedStatus  int    = http.StatusUnauthorized

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
)
