//go:build apitests

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

package tests

import (
	"crypto"
	"crypto/x509"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/ziti/controller/apierror"
)

// Test_EnrollmentToken_Certificate uses a token issued from a 3rd party IdP, usually a JWT, in order to enroll a client
// with a certificate.
func Test_EnrollmentToken_ToCertificate(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminManClient := ctx.NewEdgeManagementApi(nil)
	adminManCredentials := ctx.NewAdminCredentials()
	adminManApiSession, err := adminManClient.Authenticate(adminManCredentials, nil)
	ctx.Req.NoError(err)
	ctx.Req.NotNil(adminManApiSession)

	//auth policy allows certs
	authPolicyOnlyCerts := createAuthPolicyComponents("auth-policy-only-certs")
	authPolicyOnlyCerts.Create.Primary.Cert.Allowed = ToPtr(true)
	authPolicyOnlyCerts.Detail, err = adminManClient.CreateAuthPolicy(authPolicyOnlyCerts.Create)
	ctx.NoError(err)
	ctx.NotNil(authPolicyOnlyCerts.Detail)

	//auth policy doesn't allow certs
	authPolicyOnlyExtJwtCreate := createAuthPolicyComponents("auth-policy-only-ext-jwt-all")
	authPolicyOnlyExtJwtCreate.Create.Primary.ExtJWT.Allowed = ToPtr(true)
	authPolicyOnlyExtJwtCreate.Detail, err = adminManClient.CreateAuthPolicy(authPolicyOnlyExtJwtCreate.Create)
	ctx.NoError(err)
	ctx.NotNil(authPolicyOnlyExtJwtCreate.Detail)

	t.Run("a token exchanged for a certificate is valid if the ext jwt and auth policy allow it", func(t *testing.T) {
		t.Run("when no selectors set", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToCertValid := createExtJwtComponents("enroll-to-cert-valid-no-selectors")
			extJwtSingerEnrollToCertValid.Create.EnrollToCertEnabled = true
			extJwtSingerEnrollToCertValid.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerEnrollToCertValid.Create.EnrollAttributeClaimsSelector = ""
			extJwtSingerEnrollToCertValid.Create.ClaimsProperty = nil
			extJwtSingerEnrollToCertValid.Create.EnrollNameClaimsSelector = ""
			extJwtSingerEnrollToCertValid.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToCertValid.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToCertValid.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerEnrollToCertValid, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(creds)
			ctx.Req.NotNil(creds.Key)
			ctx.Req.NotEmpty(creds.Certs)

			t.Run("can authenticate with new client cert", func(t *testing.T) {
				ctx.testContextChanged(t)

				apiSession, err := clientApi.Authenticate(creds, nil)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(apiSession)

				t.Run("the identity has the correct attributes", func(t *testing.T) {
					ctx.testContextChanged(t)
					queryApiSession, err := clientApi.QueryCurrentApiSession()
					ctx.Req.NoError(err)
					ctx.Req.NotNil(queryApiSession)
					ctx.Req.NotNil(queryApiSession.Identity)
					ctx.Req.NotEmpty(queryApiSession.Identity.ID)

					createdIdentity, err := adminManClient.GetIdentity(queryApiSession.Identity.ID)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(createdIdentity)

					ctx.Req.Empty(*createdIdentity.RoleAttributes)
					ctx.Req.Equal(enrollClaims.Subject, *createdIdentity.Name)
					ctx.Req.Equal(enrollClaims.Subject, *createdIdentity.ExternalID)
					ctx.Req.Equal(*authPolicyOnlyCerts.Detail.ID, *createdIdentity.AuthPolicyID)
				})

				t.Run("cannot re-enroll", func(t *testing.T) {
					ctx.testContextChanged(t)
					creds, err = clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

					ctx.Req.Error(err)
					ctx.Req.ApiErrorWithCode(err, apierror.EnrollmentIdentityAlreadyEnrolledCode)
					ctx.Req.Nil(creds)
				})
			})
		})

		t.Run("when all selectors set with multiple attributes", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToCertAllSelectorsValid := createExtJwtComponents("enroll-to-cert-valid-all-selectors-multiple-attrs")
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollToCertEnabled = true
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollAttributeClaimsSelector = ClaimsWithAttributesAttributeArrayPropertyName
			extJwtSingerEnrollToCertAllSelectorsValid.Create.ClaimsProperty = ToPtr(ClaimsWithAttributesCustomIdPropertyName)
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollNameClaimsSelector = ClaimsWithAttributesCustomNamePropertyName
			extJwtSingerEnrollToCertAllSelectorsValid.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToCertAllSelectorsValid.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToCertAllSelectorsValid.Detail)

			const (
				CustomAttribute1 = "no-selectors-custom-attribute-1"
				CustomAttribute2 = "no-selectors-custom-attribute-2"
				CustomAttribute3 = "no-selectors-custom-attribute-3"
				CustomId         = "no-selectors-custom-id"
				CustomName       = "no-selectors-custom-name"
			)
			enrollClaims := &claimsWithAttributes{
				RegisteredClaims: jwt.RegisteredClaims{},
				AttributeArray: []string{
					CustomAttribute1,
					CustomAttribute2,
				},
				AttributeString: CustomAttribute3,
				CustomId:        CustomId,
				CustomName:      CustomName,
			}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerEnrollToCertAllSelectorsValid, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(creds)
			ctx.Req.NotNil(creds.Key)
			ctx.Req.NotEmpty(creds.Certs)

			t.Run("can authenticate with new client cert", func(t *testing.T) {
				ctx.testContextChanged(t)

				apiSession, err := clientApi.Authenticate(creds, nil)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(apiSession)

				t.Run("the identity has the correct attributes", func(t *testing.T) {
					ctx.testContextChanged(t)
					queryApiSession, err := clientApi.QueryCurrentApiSession()
					ctx.Req.NoError(err)
					ctx.Req.NotNil(queryApiSession)
					ctx.Req.NotNil(queryApiSession.Identity)
					ctx.Req.NotEmpty(queryApiSession.Identity.ID)

					createdIdentity, err := adminManClient.GetIdentity(queryApiSession.Identity.ID)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(createdIdentity)

					ctx.Req.NotEmpty(*createdIdentity.RoleAttributes)
					ctx.Req.ElementsMatch(enrollClaims.AttributeArray, *createdIdentity.RoleAttributes)
					ctx.Req.Equal(enrollClaims.CustomName, *createdIdentity.Name)
					ctx.Req.Equal(enrollClaims.CustomId, *createdIdentity.ExternalID)
					ctx.Req.Equal(*authPolicyOnlyCerts.Detail.ID, *createdIdentity.AuthPolicyID)
				})
			})
		})

		t.Run("when all selectors set with one attribute", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToCertAllSelectorsValid := createExtJwtComponents("enroll-to-cert-valid-all-selectors-one-attr")
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollToCertEnabled = true
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollAttributeClaimsSelector = ClaimsWithAttributesAttributeStringPropertyName
			extJwtSingerEnrollToCertAllSelectorsValid.Create.ClaimsProperty = ToPtr(ClaimsWithAttributesCustomIdPropertyName)
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollNameClaimsSelector = ClaimsWithAttributesCustomNamePropertyName
			extJwtSingerEnrollToCertAllSelectorsValid.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToCertAllSelectorsValid.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToCertAllSelectorsValid.Detail)

			const (
				CustomAttribute1 = "all-selectors-custom-attribute-1"
				CustomAttribute2 = "all-selectors-custom-attribute-2"
				CustomAttribute3 = "all-selectors-custom-attribute-3"
				CustomId         = "all-selectors-custom-id"
				CustomName       = "all-selectors-custom-name"
			)
			enrollClaims := &claimsWithAttributes{
				RegisteredClaims: jwt.RegisteredClaims{},
				AttributeArray: []string{
					CustomAttribute1,
					CustomAttribute2,
				},
				AttributeString: CustomAttribute3,
				CustomId:        CustomId,
				CustomName:      CustomName,
			}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerEnrollToCertAllSelectorsValid, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(creds)
			ctx.Req.NotNil(creds.Key)
			ctx.Req.NotEmpty(creds.Certs)

			t.Run("can authenticate with new client cert", func(t *testing.T) {
				ctx.testContextChanged(t)

				apiSession, err := clientApi.Authenticate(creds, nil)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(apiSession)

				t.Run("the identity has the correct attributes", func(t *testing.T) {
					ctx.testContextChanged(t)
					queryApiSession, err := clientApi.QueryCurrentApiSession()
					ctx.Req.NoError(err)
					ctx.Req.NotNil(queryApiSession)
					ctx.Req.NotNil(queryApiSession.Identity)
					ctx.Req.NotEmpty(queryApiSession.Identity.ID)

					createdIdentity, err := adminManClient.GetIdentity(queryApiSession.Identity.ID)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(createdIdentity)

					ctx.Req.NotEmpty(*createdIdentity.RoleAttributes)
					ctx.Req.ElementsMatch([]string{CustomAttribute3}, *createdIdentity.RoleAttributes)
					ctx.Req.Equal(enrollClaims.CustomName, *createdIdentity.Name)
					ctx.Req.Equal(enrollClaims.CustomId, *createdIdentity.ExternalID)
					ctx.Req.Equal(*authPolicyOnlyCerts.Detail.ID, *createdIdentity.AuthPolicyID)
				})
			})
		})
	})

	t.Run("a token exchanged for a certificate is invalid", func(t *testing.T) {

		t.Run("if the ext jwt is disabled", func(t *testing.T) {
			extJwtSingerDisabled := createExtJwtComponents("enroll-to-cert-disabled")
			extJwtSingerDisabled.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerDisabled.Create.Enabled = ToPtr(false)
			extJwtSingerDisabled.Create.EnrollToCertEnabled = true
			extJwtSingerDisabled.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerDisabled.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerDisabled.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerDisabled, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the name claim selector does not resolve", func(t *testing.T) {
			extJwtSingerNameSelectorFails := createExtJwtComponents("enroll-to-cert-name-selector-fails")
			extJwtSingerNameSelectorFails.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerNameSelectorFails.Create.Enabled = ToPtr(true)
			extJwtSingerNameSelectorFails.Create.EnrollNameClaimsSelector = "invalid-name-selector"
			extJwtSingerNameSelectorFails.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerNameSelectorFails.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerNameSelectorFails.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerNameSelectorFails, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the name claim selector resolves to a non-string", func(t *testing.T) {
			extJwtSingerNameIsNumberSelectorFails := createExtJwtComponents("enroll-to-cert-name-selector-is-number-fails")
			extJwtSingerNameIsNumberSelectorFails.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerNameIsNumberSelectorFails.Create.Enabled = ToPtr(true)
			extJwtSingerNameIsNumberSelectorFails.Create.EnrollToCertEnabled = true
			extJwtSingerNameIsNumberSelectorFails.Create.EnrollNameClaimsSelector = "numberValue"
			extJwtSingerNameIsNumberSelectorFails.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerNameIsNumberSelectorFails.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerNameIsNumberSelectorFails.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerNameIsNumberSelectorFails, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the attribute claim selector does not resolve", func(t *testing.T) {
			extJwtSingerAttrSelectorFails := createExtJwtComponents("enroll-to-cert-attr-selector-fails")
			extJwtSingerAttrSelectorFails.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerAttrSelectorFails.Create.Enabled = ToPtr(true)
			extJwtSingerAttrSelectorFails.Create.EnrollToCertEnabled = true
			extJwtSingerAttrSelectorFails.Create.EnrollAttributeClaimsSelector = "invalid-attr-selector"
			extJwtSingerAttrSelectorFails.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerAttrSelectorFails.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerAttrSelectorFails.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerAttrSelectorFails, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the attribute claim selector resolves to a non-string or non-string-array", func(t *testing.T) {
			extJwtSingerAttrIsNumberSelectorNotString := createExtJwtComponents("enroll-to-cert-attr-selector-is-number-fails")
			extJwtSingerAttrIsNumberSelectorNotString.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerAttrIsNumberSelectorNotString.Create.Enabled = ToPtr(true)
			extJwtSingerAttrIsNumberSelectorNotString.Create.EnrollToCertEnabled = true
			extJwtSingerAttrIsNumberSelectorNotString.Create.EnrollAttributeClaimsSelector = "numberValue"
			extJwtSingerAttrIsNumberSelectorNotString.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerAttrIsNumberSelectorNotString.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerAttrIsNumberSelectorNotString.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerAttrIsNumberSelectorNotString, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the id claim selector does not resolve", func(t *testing.T) {
			extJwtSingerIdClaimSelectorNoResolve := createExtJwtComponents("enroll-to-cert-id-claims-no-resolve")
			extJwtSingerIdClaimSelectorNoResolve.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerIdClaimSelectorNoResolve.Create.Enabled = ToPtr(true)
			extJwtSingerIdClaimSelectorNoResolve.Create.EnrollToCertEnabled = true
			extJwtSingerIdClaimSelectorNoResolve.Create.ClaimsProperty = ToPtr("i-do-not-exist")
			extJwtSingerIdClaimSelectorNoResolve.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerIdClaimSelectorNoResolve.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerIdClaimSelectorNoResolve.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerIdClaimSelectorNoResolve, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the id claim selector resolves to a non-string", func(t *testing.T) {
			extJwtSingerIdClaimSelectorNotAString := createExtJwtComponents("enroll-to-cert-id-claims-not-a-string")
			extJwtSingerIdClaimSelectorNotAString.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerIdClaimSelectorNotAString.Create.Enabled = ToPtr(true)
			extJwtSingerIdClaimSelectorNotAString.Create.EnrollToCertEnabled = true
			extJwtSingerIdClaimSelectorNotAString.Create.ClaimsProperty = ToPtr("numberValue")
			extJwtSingerIdClaimSelectorNotAString.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerIdClaimSelectorNotAString.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerIdClaimSelectorNotAString.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerIdClaimSelectorNotAString, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the ext jwt doesn't allow it", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToCertInvalid := createExtJwtComponents("enroll-to-cert-invalid-no-selectors")
			extJwtSingerEnrollToCertInvalid.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerEnrollToCertInvalid.Create.EnrollAttributeClaimsSelector = ""
			extJwtSingerEnrollToCertInvalid.Create.ClaimsProperty = nil
			extJwtSingerEnrollToCertInvalid.Create.EnrollNameClaimsSelector = ""
			extJwtSingerEnrollToCertInvalid.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToCertInvalid.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToCertInvalid.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerEnrollToCertInvalid, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentNotAllowedCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the ext jwt allows it but the auth policy doesn't", func(t *testing.T) {
			ctx.testContextChanged(t)

			authPolicyCertsAtFirstThenNo := createAuthPolicyComponents("auth-policy-certs-at-first-then-no")
			authPolicyCertsAtFirstThenNo.Create.Primary.Cert.Allowed = ToPtr(true)
			authPolicyCertsAtFirstThenNo.Detail, err = adminManClient.CreateAuthPolicy(authPolicyCertsAtFirstThenNo.Create)
			ctx.NoError(err)
			ctx.NotNil(authPolicyCertsAtFirstThenNo.Detail)

			extJwtSingerAuthPolicyNoCerts := createExtJwtComponents("enroll-to-cert-invalid-auth-policy-no-certs")
			extJwtSingerAuthPolicyNoCerts.Create.EnrollToCertEnabled = true
			extJwtSingerAuthPolicyNoCerts.Create.EnrollAuthPolicyID = *authPolicyCertsAtFirstThenNo.Detail.ID
			extJwtSingerAuthPolicyNoCerts.Create.EnrollAttributeClaimsSelector = ClaimsWithAttributesAttributeArrayPropertyName
			extJwtSingerAuthPolicyNoCerts.Create.ClaimsProperty = ToPtr(ClaimsWithAttributesCustomIdPropertyName)
			extJwtSingerAuthPolicyNoCerts.Create.EnrollNameClaimsSelector = ClaimsWithAttributesCustomNamePropertyName
			extJwtSingerAuthPolicyNoCerts.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerAuthPolicyNoCerts.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerAuthPolicyNoCerts.Detail)

			//update the auth policy to no longer allow certs
			authPolicyPatch := &rest_model.AuthPolicyPatch{
				Primary: &rest_model.AuthPolicyPrimaryPatch{
					Cert: &rest_model.AuthPolicyPrimaryCertPatch{
						Allowed: ToPtr(false),
					},
				},
			}
			authPolicyCertsAtFirstThenNo.Detail, err = adminManClient.PatchExtJwtSigner(*authPolicyCertsAtFirstThenNo.Detail.ID, authPolicyPatch)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(authPolicyCertsAtFirstThenNo.Detail)

			const (
				CustomAttribute1 = "invalid-ap-no-certs-custom-attribute-1"
				CustomAttribute2 = "invalid-ap-no-certs-attribute-2"
				CustomAttribute3 = "invalid-ap-no-certs-attribute-3"
				CustomId         = "invalid-ap-no-certs-id"
				CustomName       = "invalid-ap-no-certs-name"
			)
			enrollClaims := &claimsWithAttributes{
				RegisteredClaims: jwt.RegisteredClaims{},
				AttributeArray: []string{
					CustomAttribute1,
					CustomAttribute2,
				},
				AttributeString: CustomAttribute3,
				CustomId:        CustomId,
				CustomName:      CustomName,
			}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerAuthPolicyNoCerts, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentNotAllowedCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the token is missing", func(t *testing.T) {
			ctx.testContextChanged(t)
			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth("")

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the token is malformed", func(t *testing.T) {
			ctx.testContextChanged(t)
			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth("fjgneonberoinoeribnreoinerboerinobianroiban")

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})

		t.Run("if the token is from another signer", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToCertValid := createExtJwtComponents("enroll-to-cert-valid-right-singer")
			extJwtSingerEnrollToCertValid.Create.EnrollToCertEnabled = true
			extJwtSingerEnrollToCertValid.Create.EnrollAuthPolicyID = *authPolicyOnlyCerts.Detail.ID
			extJwtSingerEnrollToCertValid.Create.EnrollAttributeClaimsSelector = ""
			extJwtSingerEnrollToCertValid.Create.ClaimsProperty = nil
			extJwtSingerEnrollToCertValid.Create.EnrollNameClaimsSelector = ""
			extJwtSingerEnrollToCertValid.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToCertValid.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToCertValid.Detail)

			//not actually created so that the JWT doesn't select this ext jwt signer, otherwise it will just fail
			//due to this ext jwt signer not allowing enrollment.
			extJwtSingerEnrollToCertWrongSigner := createExtJwtComponents("enroll-to-cert-valid-wrong-singer")
			extJwtSingerEnrollToCertWrongSigner.Detail = &rest_model.ExternalJWTSignerDetail{
				Kid:      extJwtSingerEnrollToCertWrongSigner.Create.Kid,
				Issuer:   extJwtSingerEnrollToCertWrongSigner.Create.Issuer,
				Audience: extJwtSingerEnrollToCertWrongSigner.Create.Audience,
			}

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerEnrollToCertWrongSigner, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
			ctx.Req.Nil(creds)
		})
	})
}

type extJwtSignerComponents struct {
	Name   string
	Issuer string

	CertCommonName string
	Cert           *x509.Certificate
	Key            crypto.PrivateKey
	CertPem        string

	Create *rest_model.ExternalJWTSignerCreate
	Detail *rest_model.ExternalJWTSignerDetail
}

type authPolicyComponents struct {
	Create *rest_model.AuthPolicyCreate
	Detail *rest_model.AuthPolicyDetail
}

// createsExtJwtComponent creates a basic ext jwt signer with default values based on the given name.
func createExtJwtComponents(name string) *extJwtSignerComponents {
	result := &extJwtSignerComponents{}

	result.CertCommonName = name
	result.Issuer = name + "-issuer"
	result.Cert, result.Key = newSelfSignedCert(name)
	result.CertPem = nfpem.EncodeToString(result.Cert)

	result.Create = &rest_model.ExternalJWTSignerCreate{
		CertPem:         ToPtr(result.CertPem),
		ClaimsProperty:  ToPtr(name + "-id-claims-property"),
		Enabled:         ToPtr(true),
		ExternalAuthURL: ToPtr("https://" + name + "/auth"),
		Name:            ToPtr(name),
		UseExternalID:   ToPtr(true),
		Kid:             ToPtr(name + "-kid"),
		Issuer:          ToPtr(name + "-issuer"),
		Audience:        ToPtr(name + "-audience"),
		ClientID:        ToPtr(name + "-client-id"),
		Scopes:          []string{name + "-scope1", name + "-scope2"},
		TargetToken:     ToPtr(rest_model.TargetTokenID),
	}

	return result
}

func createAuthPolicyComponents(name string) *authPolicyComponents {
	return &authPolicyComponents{
		Create: &rest_model.AuthPolicyCreate{
			Name: ToPtr(name),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					Allowed:           ToPtr(false),
					AllowExpiredCerts: ToPtr(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(false),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(false),
					LockoutDurationMinutes: ToPtr(int64(5)),
					MaxAttempts:            ToPtr(int64(5)),
					MinPasswordLength:      ToPtr(int64(5)),
					RequireMixedCase:       ToPtr(true),
					RequireNumberChar:      ToPtr(true),
					RequireSpecialChar:     ToPtr(true),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: nil,
				RequireTotp:         ToPtr(false),
			},
		},
	}
}

func newJwtForExtJwtSigner(extJwtComponents *extJwtSignerComponents, claims *claimsWithAttributes) (string, error) {
	jwtToken := jwt.New(jwt.SigningMethodES256)

	claims.RegisteredClaims = jwt.RegisteredClaims{
		Audience:  []string{*extJwtComponents.Detail.Audience},
		ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
		ID:        time.Now().String(),
		IssuedAt:  &jwt.NumericDate{Time: time.Now()},
		Issuer:    extJwtComponents.Issuer,
		NotBefore: &jwt.NumericDate{Time: time.Now()},
		Subject:   uuid.NewString(),
	}

	jwtToken.Header["kid"] = *extJwtComponents.Detail.Kid
	jwtToken.Claims = claims

	return jwtToken.SignedString(extJwtComponents.Key)
}

const (
	ClaimsWithAttributesAttributeArrayPropertyName  = "attributeArray"
	ClaimsWithAttributesAttributeStringPropertyName = "attributeString"
	ClaimsWithAttributesCustomIdPropertyName        = "customId"
	ClaimsWithAttributesCustomNamePropertyName      = "customName"
)

type claimsWithAttributes struct {
	jwt.RegisteredClaims
	AttributeArray  []string `json:"attributeArray,omitempty"`
	AttributeString string   `json:"attributeString,omitempty"`
	CustomId        string   `json:"customId,omitempty"`
	CustomName      string   `json:"customName,omitempty"`
	NumberValue     int64    `json:"numberValue,omitempty"`
}
