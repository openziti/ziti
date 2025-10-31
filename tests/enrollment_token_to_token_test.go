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
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/edge-api/rest_model"
	edgeApis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/controller/apierror"
)

// Test_EnrollmentToken_Certificate uses a token issued from a 3rd party IdP, usually a JWT, in order to enroll a client
// with a certificate.
func Test_EnrollmentToken_ToToken(t *testing.T) {
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

	//auth policy doesn't allow certs, but allows all ext jwts
	authPolicyOnlyExtJwtCreate := createAuthPolicyComponents("auth-policy-only-ext-jwt-all")
	authPolicyOnlyExtJwtCreate.Create.Primary.ExtJWT.Allowed = ToPtr(true)
	authPolicyOnlyExtJwtCreate.Detail, err = adminManClient.CreateAuthPolicy(authPolicyOnlyExtJwtCreate.Create)
	ctx.NoError(err)
	ctx.NotNil(authPolicyOnlyExtJwtCreate.Detail)

	t.Run("a token exchanged for token enrollment is valid if the ext jwt and auth policy allow it", func(t *testing.T) {
		t.Run("when no selectors set", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToTokenValid := createExtJwtComponents("enroll-to-token-valid-no-selectors")
			extJwtSingerEnrollToTokenValid.Create.EnrollToTokenEnabled = true
			extJwtSingerEnrollToTokenValid.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerEnrollToTokenValid.Create.EnrollAttributeClaimsSelector = ""
			extJwtSingerEnrollToTokenValid.Create.ClaimsProperty = nil
			extJwtSingerEnrollToTokenValid.Create.EnrollNameClaimsSelector = ""
			extJwtSingerEnrollToTokenValid.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToTokenValid.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToTokenValid.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerEnrollToTokenValid, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)
			ctx.Req.NoError(err)

			t.Run("can authenticate with token", func(t *testing.T) {
				ctx.testContextChanged(t)

				//enrollment JWT should also be enough to authenticate
				creds := edgeApis.NewJwtCredentials(enrollmentJwt)

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
					ctx.Req.Equal(*authPolicyOnlyExtJwtCreate.Detail.ID, *createdIdentity.AuthPolicyID)
				})

				t.Run("cannot re-enroll", func(t *testing.T) {
					ctx.testContextChanged(t)
					err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)

					ctx.Req.Error(err)
				})
			})
		})

		t.Run("when all selectors set with multiple attributes", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToCertAllSelectorsValid := createExtJwtComponents("enroll-to-token-valid-all-selectors-multiple-attrs")
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollToTokenEnabled = true
			extJwtSingerEnrollToCertAllSelectorsValid.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
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

			err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)

			ctx.Req.NoError(err)

			t.Run("can authenticate with new client cert", func(t *testing.T) {
				ctx.testContextChanged(t)

				creds := edgeApis.NewJwtCredentials(enrollmentJwt)
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
					ctx.Req.Equal(*authPolicyOnlyExtJwtCreate.Detail.ID, *createdIdentity.AuthPolicyID)
				})
			})
		})

		t.Run("when all selectors set with one attribute", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToTokenAllSelectorsValid := createExtJwtComponents("enroll-to-token-valid-all-selectors-one-attr")
			extJwtSingerEnrollToTokenAllSelectorsValid.Create.EnrollToTokenEnabled = true
			extJwtSingerEnrollToTokenAllSelectorsValid.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerEnrollToTokenAllSelectorsValid.Create.EnrollAttributeClaimsSelector = ClaimsWithAttributesAttributeStringPropertyName
			extJwtSingerEnrollToTokenAllSelectorsValid.Create.ClaimsProperty = ToPtr(ClaimsWithAttributesCustomIdPropertyName)
			extJwtSingerEnrollToTokenAllSelectorsValid.Create.EnrollNameClaimsSelector = ClaimsWithAttributesCustomNamePropertyName
			extJwtSingerEnrollToTokenAllSelectorsValid.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToTokenAllSelectorsValid.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToTokenAllSelectorsValid.Detail)

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
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerEnrollToTokenAllSelectorsValid, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)

			ctx.Req.NoError(err)

			t.Run("can authenticate with new client cert", func(t *testing.T) {
				ctx.testContextChanged(t)

				creds := edgeApis.NewJwtCredentials(enrollmentJwt)
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
					ctx.Req.Equal(*authPolicyOnlyExtJwtCreate.Detail.ID, *createdIdentity.AuthPolicyID)
				})
			})
		})
	})

	t.Run("a token for token auth is invalid", func(t *testing.T) {

		t.Run("if the ext jwt is disabled", func(t *testing.T) {
			extJwtSingerDisabled := createExtJwtComponents("enroll-to-token-disabled")
			extJwtSingerDisabled.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerDisabled.Create.Enabled = ToPtr(false)
			extJwtSingerDisabled.Create.EnrollToTokenEnabled = true
			extJwtSingerDisabled.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerDisabled.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerDisabled.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerDisabled, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)
			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
		})

		t.Run("if the name claim selector does not resolve", func(t *testing.T) {
			extJwtSingerNameSelectorFails := createExtJwtComponents("enroll-to-token-name-selector-fails")
			extJwtSingerNameSelectorFails.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
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
			extJwtSingerNameIsNumberSelectorFails := createExtJwtComponents("enroll-to-token-name-selector-is-number-fails")
			extJwtSingerNameIsNumberSelectorFails.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerNameIsNumberSelectorFails.Create.Enabled = ToPtr(true)
			extJwtSingerNameIsNumberSelectorFails.Create.EnrollToTokenEnabled = true
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
			extJwtSingerAttrSelectorFails := createExtJwtComponents("enroll-to-token-attr-selector-fails")
			extJwtSingerAttrSelectorFails.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerAttrSelectorFails.Create.Enabled = ToPtr(true)
			extJwtSingerAttrSelectorFails.Create.EnrollToTokenEnabled = true
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

			err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
		})

		t.Run("if the attribute claim selector resolves to a non-string or non-string-array", func(t *testing.T) {
			extJwtSingerAttrIsNumberSelectorNotString := createExtJwtComponents("enroll-to-token-attr-selector-is-number-fails")
			extJwtSingerAttrIsNumberSelectorNotString.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerAttrIsNumberSelectorNotString.Create.Enabled = ToPtr(true)
			extJwtSingerAttrIsNumberSelectorNotString.Create.EnrollToTokenEnabled = true
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
			extJwtSingerIdClaimSelectorNoResolve := createExtJwtComponents("enroll-to-token-id-claims-no-resolve")
			extJwtSingerIdClaimSelectorNoResolve.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerIdClaimSelectorNoResolve.Create.Enabled = ToPtr(true)
			extJwtSingerIdClaimSelectorNoResolve.Create.EnrollToTokenEnabled = true
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

			err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
		})

		t.Run("if the id claim selector resolves to a non-string", func(t *testing.T) {
			extJwtSingerIdClaimSelectorNotAString := createExtJwtComponents("enroll-to-token-id-claims-not-a-string")
			extJwtSingerIdClaimSelectorNotAString.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerIdClaimSelectorNotAString.Create.Enabled = ToPtr(true)
			extJwtSingerIdClaimSelectorNotAString.Create.EnrollToTokenEnabled = true
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

			err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.ApiErrorWithCode(err, apierror.InvalidEnrollmentTokenCode)
		})

		t.Run("if the ext jwt doesn't allow it", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToTokenInvalid := createExtJwtComponents("enroll-to-token-invalid-no-selectors")
			extJwtSingerEnrollToTokenInvalid.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerEnrollToTokenInvalid.Create.EnrollAttributeClaimsSelector = ""
			extJwtSingerEnrollToTokenInvalid.Create.ClaimsProperty = nil
			extJwtSingerEnrollToTokenInvalid.Create.EnrollNameClaimsSelector = ""
			extJwtSingerEnrollToTokenInvalid.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToTokenInvalid.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToTokenInvalid.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerEnrollToTokenInvalid, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			creds, err := clientApi.CompleteJwtTokenEnrollmentToCertAuth(enrollmentJwt)

			ctx.Req.Error(err)
			ctx.Req.Nil(creds)
		})

		t.Run("if the ext jwt allows it but the auth policy doesn't", func(t *testing.T) {
			ctx.testContextChanged(t)

			authPolicyTokensAtFirstThenNo := createAuthPolicyComponents("auth-policy-tokens-at-first-then-no")
			authPolicyTokensAtFirstThenNo.Create.Primary.ExtJWT.Allowed = ToPtr(true)
			authPolicyTokensAtFirstThenNo.Detail, err = adminManClient.CreateAuthPolicy(authPolicyTokensAtFirstThenNo.Create)
			ctx.NoError(err)
			ctx.NotNil(authPolicyTokensAtFirstThenNo.Detail)

			extJwtSingerAuthPolicyNoTokens := createExtJwtComponents("enroll-to-token-invalid-auth-policy-no-tokens")
			extJwtSingerAuthPolicyNoTokens.Create.EnrollToTokenEnabled = true
			extJwtSingerAuthPolicyNoTokens.Create.EnrollAuthPolicyID = *authPolicyTokensAtFirstThenNo.Detail.ID
			extJwtSingerAuthPolicyNoTokens.Create.EnrollAttributeClaimsSelector = ClaimsWithAttributesAttributeArrayPropertyName
			extJwtSingerAuthPolicyNoTokens.Create.ClaimsProperty = ToPtr(ClaimsWithAttributesCustomIdPropertyName)
			extJwtSingerAuthPolicyNoTokens.Create.EnrollNameClaimsSelector = ClaimsWithAttributesCustomNamePropertyName
			extJwtSingerAuthPolicyNoTokens.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerAuthPolicyNoTokens.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerAuthPolicyNoTokens.Detail)

			//update the auth policy to no longer allow certs
			authPolicyPatchToNoTokens := &rest_model.AuthPolicyPatch{
				Primary: &rest_model.AuthPolicyPrimaryPatch{
					ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
						Allowed: ToPtr(false),
					},
				},
			}
			authPolicyTokensAtFirstThenNo.Detail, err = adminManClient.PatchExtJwtSigner(*authPolicyTokensAtFirstThenNo.Detail.ID, authPolicyPatchToNoTokens)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(authPolicyTokensAtFirstThenNo.Detail)

			const (
				CustomAttribute1 = "invalid-ap-no-tokens-custom-attribute-1"
				CustomAttribute2 = "invalid-ap-no-tokens-attribute-2"
				CustomAttribute3 = "invalid-ap-no-tokens-attribute-3"
				CustomId         = "invalid-ap-no-tokens-id"
				CustomName       = "invalid-ap-no-tokens-name"
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
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerAuthPolicyNoTokens, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)

			ctx.Req.Error(err)
		})

		t.Run("if the token is missing", func(t *testing.T) {
			ctx.testContextChanged(t)
			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			err := clientApi.CompleteJwtTokenEnrollmentToTokenAuth("")

			ctx.Req.Error(err)
		})

		t.Run("if the token is malformed", func(t *testing.T) {
			ctx.testContextChanged(t)
			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			err := clientApi.CompleteJwtTokenEnrollmentToTokenAuth("eYsdfgsdgfd.dfgsfdgfds.sdfgsdfg")

			ctx.Req.Error(err)
		})

		t.Run("if the token is from another signer", func(t *testing.T) {
			ctx.testContextChanged(t)

			extJwtSingerEnrollToTokenValid := createExtJwtComponents("enroll-to-token-valid-signer")
			extJwtSingerEnrollToTokenValid.Create.EnrollToTokenEnabled = true
			extJwtSingerEnrollToTokenValid.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerEnrollToTokenValid.Create.EnrollAttributeClaimsSelector = ""
			extJwtSingerEnrollToTokenValid.Create.ClaimsProperty = nil
			extJwtSingerEnrollToTokenValid.Create.EnrollNameClaimsSelector = ""
			extJwtSingerEnrollToTokenValid.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToTokenValid.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToTokenValid.Detail)

			extJwtSingerEnrollToTokenWrongSigner := createExtJwtComponents("enroll-to-token-invalid-wrong-singer")
			extJwtSingerEnrollToTokenWrongSigner.Create.EnrollAuthPolicyID = *authPolicyOnlyExtJwtCreate.Detail.ID
			extJwtSingerEnrollToTokenWrongSigner.Create.EnrollAttributeClaimsSelector = ""
			extJwtSingerEnrollToTokenWrongSigner.Create.ClaimsProperty = nil
			extJwtSingerEnrollToTokenWrongSigner.Create.EnrollNameClaimsSelector = ""
			extJwtSingerEnrollToTokenWrongSigner.Detail, err = adminManClient.CreateExtJwtSigner(extJwtSingerEnrollToTokenWrongSigner.Create)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(extJwtSingerEnrollToTokenWrongSigner.Detail)

			enrollClaims := &claimsWithAttributes{}
			enrollmentJwt, err := newJwtForExtJwtSigner(extJwtSingerEnrollToTokenWrongSigner, enrollClaims)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(enrollmentJwt)

			clientApi := ctx.NewEdgeClientApi(nil)
			ctx.Req.NotNil(clientApi)

			err = clientApi.CompleteJwtTokenEnrollmentToTokenAuth(enrollmentJwt)

			ctx.Req.Error(err)
		})
	})
}
