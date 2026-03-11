//go:build apitests

package tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	authenticator2 "github.com/openziti/edge-api/rest_management_api_client/authenticator"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	identity2 "github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	nfpem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/oidc_auth"
)

func Test_Authenticate_OIDC_Auth(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	clientHelper := ctx.NewEdgeClientApi(nil)

	managementHelper := ctx.NewEdgeManagementApi(nil)
	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminCreds.CaPool = ctx.ControllerCaPool()
	_, err := managementHelper.Authenticate(adminCreds, nil)
	ctx.Req.NoError(rest_util.WrapErr(err))

	t.Run("attempt to auth with multipart form data, expect unsupported media type", func(t *testing.T) {
		ctx.NextTest(t)

		client := resty.NewWithClient(ctx.NewHttpClient(ctx.NewTransport()))

		loginPath := "https://" + ctx.ApiHost + "/oidc/login/password?authRequestID=12345"

		resp, err := client.R().SetMultipartFormData(map[string]string{
			"username": "admin",
			"password": "admin",
		}).Post(loginPath)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnsupportedMediaType, resp.StatusCode())
	})

	t.Run("updb auth", func(t *testing.T) {
		ctx.NextTest(t)

		tokens, _, err := clientHelper.RawOidcAuthRequest(adminCreds)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.IDToken)
		ctx.Req.NotEmpty(tokens.AccessToken)
		ctx.Req.NotEmpty(tokens.RefreshToken)

		t.Run("access token has expected values", func(t *testing.T) {
			ctx.NextTest(t)
			parser := jwt.NewParser()

			accessClaims := &common.AccessClaims{}

			_, _, err := parser.ParseUnverified(tokens.AccessToken, accessClaims)

			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.False(accessClaims.IsCertExtendable)
			ctx.Req.True(accessClaims.IsAdmin)
			ctx.Req.NotEmpty(accessClaims.ApiSessionId)
			ctx.Req.NotEmpty(accessClaims.JWTID)
			ctx.Req.Equal(common.TokenTypeAccess, accessClaims.Type)
			ctx.Req.NotEmpty(accessClaims.Subject)
			ctx.Req.Contains(accessClaims.AuthenticationMethodsReferences, oidc_auth.AuthMethodPassword)
			ctx.Req.NotZero(accessClaims.AuthTime)
		})
	})

	t.Run("updb with auth request id in query string", func(t *testing.T) {
		ctx.NextTest(t)

		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/username?authRequestID=" + result.AuthRequestId

		resp, err := result.Client.R().SetFormData(map[string]string{
			"username": ctx.AdminAuthenticator.Username,
			"password": ctx.AdminAuthenticator.Password,
		}).Post(opLoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusFound, resp.StatusCode())

		locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
		ctx.Req.NoError(parseErr)
		code := locUrl.Query().Get("code")
		ctx.Req.NotEmpty(code)

		tokens, err := result.Exchange(code)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)

		t.Run("access token has expected values", func(t *testing.T) {
			ctx.NextTest(t)
			parser := jwt.NewParser()

			accessClaims := &common.AccessClaims{}

			_, _, err := parser.ParseUnverified(tokens.AccessToken, accessClaims)

			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.False(accessClaims.IsCertExtendable)
			ctx.Req.True(accessClaims.IsAdmin)
			ctx.Req.NotEmpty(accessClaims.ApiSessionId)
			ctx.Req.NotEmpty(accessClaims.JWTID)
			ctx.Req.Equal(common.TokenTypeAccess, accessClaims.Type)
			ctx.Req.NotEmpty(accessClaims.Subject)
			ctx.Req.Contains(accessClaims.AuthenticationMethodsReferences, oidc_auth.AuthMethodPassword)
			ctx.Req.NotZero(accessClaims.AuthTime)
		})
	})

	t.Run("updb with id in query string", func(t *testing.T) {
		ctx.NextTest(t)

		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/username?id=" + result.AuthRequestId

		payload := &oidc_auth.OidcUpdbCreds{
			Authenticate: rest_model.Authenticate{
				EnvInfo: &rest_model.EnvInfo{
					Arch:      "ARCH1",
					Domain:    "DOMAIN1",
					Hostname:  "HOSTNAME1",
					Os:        "OS1",
					OsRelease: "OSRELEASE1",
					OsVersion: "1.1.1",
				},
				Password: rest_model.Password(ctx.AdminAuthenticator.Password),
				SdkInfo: &rest_model.SdkInfo{
					AppID:      "APPID1",
					AppVersion: "2.2.2",
					Branch:     "BRANCH1",
					Revision:   "REVISION1",
					Type:       "TEST1",
					Version:    "3.3.3",
				},
				Username: rest_model.Username(ctx.AdminAuthenticator.Username),
			},
			AuthRequestBody: oidc_auth.AuthRequestBody{
				AuthRequestId: result.AuthRequestId,
			},
		}

		resp, err := result.Client.R().SetBody(payload).Post(opLoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusFound, resp.StatusCode())

		locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
		ctx.Req.NoError(parseErr)
		code := locUrl.Query().Get("code")
		ctx.Req.NotEmpty(code)

		tokens, err := result.Exchange(code)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)

		t.Run("access token has expected values", func(t *testing.T) {
			ctx.NextTest(t)
			parser := jwt.NewParser()

			accessClaims := &common.AccessClaims{}

			_, _, err := parser.ParseUnverified(tokens.AccessToken, accessClaims)

			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.False(accessClaims.IsCertExtendable)
			ctx.Req.True(accessClaims.IsAdmin)
			ctx.Req.NotEmpty(accessClaims.ApiSessionId)
			ctx.Req.NotEmpty(accessClaims.JWTID)
			ctx.Req.Equal(common.TokenTypeAccess, accessClaims.Type)
			ctx.Req.NotEmpty(accessClaims.Subject)
			ctx.Req.Contains(accessClaims.AuthenticationMethodsReferences, oidc_auth.AuthMethodPassword)
			ctx.Req.NotZero(accessClaims.AuthTime)

			t.Run("has the correct sdk and env info", func(t *testing.T) {
				ctx.testContextChanged(t)

				time.Sleep(time.Second)
				identityDetail, err := managementHelper.GetIdentity(accessClaims.Subject)

				ctx.Req.NoError(err)

				ctx.Req.Equal(payload.SdkInfo.AppID, identityDetail.SdkInfo.AppID)
				ctx.Req.Equal(payload.SdkInfo.AppVersion, identityDetail.SdkInfo.AppVersion)
				ctx.Req.Equal(payload.SdkInfo.Branch, identityDetail.SdkInfo.Branch)
				ctx.Req.Equal(payload.SdkInfo.Revision, identityDetail.SdkInfo.Revision)
				ctx.Req.Equal(payload.SdkInfo.Type, identityDetail.SdkInfo.Type)
				ctx.Req.Equal(payload.SdkInfo.Version, identityDetail.SdkInfo.Version)

				ctx.Req.Equal(payload.EnvInfo.Arch, identityDetail.EnvInfo.Arch)
				ctx.Req.Equal(payload.EnvInfo.Domain, identityDetail.EnvInfo.Domain)
				ctx.Req.Equal(payload.EnvInfo.Hostname, identityDetail.EnvInfo.Hostname)
				ctx.Req.Equal(payload.EnvInfo.Os, identityDetail.EnvInfo.Os)
				ctx.Req.Equal(payload.EnvInfo.OsRelease, identityDetail.EnvInfo.OsRelease)
				ctx.Req.Equal(payload.EnvInfo.OsVersion, identityDetail.EnvInfo.OsVersion)
			})
		})
	})

	t.Run("updb with invalid password", func(t *testing.T) {
		ctx.NextTest(t)

		result, err := clientHelper.OidcAuthorize(adminCreds)
		ctx.Req.NoError(err)

		opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/username?id=" + result.AuthRequestId

		resp, err := result.Client.R().SetFormData(map[string]string{
			"username": ctx.AdminAuthenticator.Username,
			"password": "invalid",
		}).
			SetHeader("Accept", "application/json").
			Post(opLoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
		ctx.Req.Equal("application/json", resp.Header().Get("Content-Type"))
	})

	t.Run("cert", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.RequireAdminManagementApiLogin()

		_, certAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment("test", false)

		certCreds := edge_apis.NewCertCredentials(certAuth.certs, certAuth.key)
		certCreds.CaPool = ctx.ControllerCaPool()

		result, err := clientHelper.OidcAuthorize(certCreds)
		ctx.Req.NoError(err)

		opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/cert"

		payload := &oidc_auth.OidcUpdbCreds{
			Authenticate: rest_model.Authenticate{
				EnvInfo: &rest_model.EnvInfo{
					Arch:      "ARCH1",
					Domain:    "DOMAIN1",
					Hostname:  "HOSTNAME1",
					Os:        "OS1",
					OsRelease: "OSRELEASE1",
					OsVersion: "1.1.1",
				},
				SdkInfo: &rest_model.SdkInfo{
					AppID:      "APPID1",
					AppVersion: "2.2.2",
					Branch:     "BRANCH1",
					Revision:   "REVISION1",
					Type:       "TEST1",
					Version:    "3.3.3",
				},
			},
			AuthRequestBody: oidc_auth.AuthRequestBody{
				AuthRequestId: result.AuthRequestId,
			},
		}

		resp, err := result.Client.R().SetBody(payload).Post(opLoginUri)

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusFound, resp.StatusCode())

		locUrl, parseErr := url.Parse(resp.Header().Get("Location"))
		ctx.Req.NoError(parseErr)
		code := locUrl.Query().Get("code")
		ctx.Req.NotEmpty(code)

		tokens, err := result.Exchange(code)
		ctx.Req.NoError(err)
		ctx.Req.NotNil(tokens)
		ctx.Req.NotEmpty(tokens.AccessToken)

		t.Run("access token has expected values", func(t *testing.T) {
			ctx.NextTest(t)
			parser := jwt.NewParser()

			accessClaims := &common.AccessClaims{}

			_, _, err := parser.ParseUnverified(tokens.AccessToken, accessClaims)

			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(accessClaims.AuthenticatorId)
			ctx.Req.True(accessClaims.IsCertExtendable)
			ctx.Req.False(accessClaims.IsAdmin)
			ctx.Req.NotEmpty(accessClaims.ApiSessionId)
			ctx.Req.NotEmpty(accessClaims.JWTID)
			ctx.Req.Equal(common.TokenTypeAccess, accessClaims.Type)
			ctx.Req.NotEmpty(accessClaims.Subject)
			ctx.Req.Contains(accessClaims.AuthenticationMethodsReferences, oidc_auth.AuthMethodCert)
			ctx.Req.NotZero(accessClaims.AuthTime)

			t.Run("has the correct sdk and env info", func(t *testing.T) {
				ctx.testContextChanged(t)

				identityDetail, err := managementHelper.GetIdentity(accessClaims.Subject)

				ctx.Req.NoError(err)

				ctx.Req.Equal(payload.SdkInfo.AppID, identityDetail.SdkInfo.AppID)
				ctx.Req.Equal(payload.SdkInfo.AppVersion, identityDetail.SdkInfo.AppVersion)
				ctx.Req.Equal(payload.SdkInfo.Branch, identityDetail.SdkInfo.Branch)
				ctx.Req.Equal(payload.SdkInfo.Revision, identityDetail.SdkInfo.Revision)
				ctx.Req.Equal(payload.SdkInfo.Type, identityDetail.SdkInfo.Type)
				ctx.Req.Equal(payload.SdkInfo.Version, identityDetail.SdkInfo.Version)

				ctx.Req.Equal(payload.EnvInfo.Arch, identityDetail.EnvInfo.Arch)
				ctx.Req.Equal(payload.EnvInfo.Domain, identityDetail.EnvInfo.Domain)
				ctx.Req.Equal(payload.EnvInfo.Hostname, identityDetail.EnvInfo.Hostname)
				ctx.Req.Equal(payload.EnvInfo.Os, identityDetail.EnvInfo.Os)
				ctx.Req.Equal(payload.EnvInfo.OsRelease, identityDetail.EnvInfo.OsRelease)
				ctx.Req.Equal(payload.EnvInfo.OsVersion, identityDetail.EnvInfo.OsVersion)
			})
		})

	})

	t.Run("test cert auth totp ext-jwt", func(t *testing.T) {
		ctx.NextTest(t)

		jwtSignerCert, _ := newSelfSignedCert("Test Jwt Signer Cert - Auth Policy")

		clientId := "test-client-id-99"
		scope1 := "test-scope-1-99"
		scope2 := "test-scope-2-99"
		extAuthUrl := "https://some.auth.url.example.com/auth"
		createExtJwtParam := external_jwt_signer.NewCreateExternalJWTSignerParams()
		createExtJwtParam.ExternalJWTSigner = &rest_model.ExternalJWTSignerCreate{
			CertPem:         ToPtr(nfpem.EncodeToString(jwtSignerCert)),
			Enabled:         ToPtr(true),
			Name:            ToPtr("Test JWT Signer - Auth Policy"),
			Kid:             ToPtr(uuid.NewString()),
			Issuer:          ToPtr("test-issuer-99"),
			Audience:        ToPtr("test-audience-99"),
			ClientID:        &clientId,
			Scopes:          []string{scope1, scope2},
			ExternalAuthURL: ToPtr(extAuthUrl),
		}

		extJwtCreateResp, err := managementHelper.API.ExternalJWTSigner.CreateExternalJWTSigner(createExtJwtParam, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(extJwtCreateResp)

		createAuthPolicyParams := auth_policy.NewCreateAuthPolicyParams()
		createAuthPolicyParams.AuthPolicy = &rest_model.AuthPolicyCreate{
			Name: ToPtr("auth_oidc_test-" + eid.New()),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: ToPtr(true),
					Allowed:           ToPtr(true),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        ToPtr(false),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                ToPtr(true),
					LockoutDurationMinutes: ToPtr(int64(0)),
					MaxAttempts:            ToPtr(int64(5)),
					MinPasswordLength:      ToPtr(int64(5)),
					RequireMixedCase:       ToPtr(false),
					RequireNumberChar:      ToPtr(false),
					RequireSpecialChar:     ToPtr(false),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: ToPtr(extJwtCreateResp.Payload.Data.ID),
				RequireTotp:         ToPtr(true),
			},
		}

		authPolicyCreateResp, err := managementHelper.API.AuthPolicy.CreateAuthPolicy(createAuthPolicyParams, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(authPolicyCreateResp)

		identityName := eid.New()
		identityExternalId := eid.New()
		createIdentityParams := identity2.NewCreateIdentityParams()
		createIdentityParams.Identity = &rest_model.IdentityCreate{
			AuthPolicyID: ToPtr(authPolicyCreateResp.Payload.Data.ID),
			ExternalID:   ToPtr(identityExternalId),
			IsAdmin:      ToPtr(false),
			Name:         ToPtr(identityName),
			Type:         ToPtr(rest_model.IdentityTypeDefault),
		}

		createIdentityResp, err := managementHelper.API.Identity.CreateIdentity(createIdentityParams, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(createIdentityResp)

		identityPassword := eid.New()

		createIdentityUpdbAuthenticator := authenticator2.NewCreateAuthenticatorParams()
		createIdentityUpdbAuthenticator.Authenticator = &rest_model.AuthenticatorCreate{
			CertPem:    "",
			IdentityID: ToPtr(createIdentityResp.Payload.Data.ID),
			Method:     ToPtr("updb"),
			Password:   identityPassword,
			Username:   identityName,
		}

		createIdentityUpdbAuthenticatorResp, err := managementHelper.API.Authenticator.CreateAuthenticator(createIdentityUpdbAuthenticator, nil)
		ctx.Req.NoError(rest_util.WrapErr(err))
		ctx.Req.NotNil(createIdentityUpdbAuthenticatorResp)

		identityCreds := edge_apis.NewUpdbCredentials(identityName, identityPassword)
		identityCreds.CaPool = ctx.ControllerCaPool()

		t.Run("can authenticate via UPDB and see two auth queries", func(t *testing.T) {
			ctx.NextTest(t)

			result, err := clientHelper.OidcAuthorize(identityCreds)
			ctx.Req.NoError(err)

			opLoginUri := "https://" + ctx.ApiHost + "/oidc/login/username"

			resp, err := result.Client.R().SetHeader("content-type", "application/json").
				SetBody(map[string]string{"id": result.AuthRequestId, "username": identityName, "password": identityPassword}).
				Post(opLoginUri)

			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			type respBody struct {
				AuthQueries []*rest_model.AuthQueryDetail `json:"authQueries"`
			}

			parsedBody := &respBody{}

			err = json.Unmarshal(resp.Body(), parsedBody)
			ctx.Req.NoError(err)

			ctx.Req.Len(parsedBody.AuthQueries, 2)

			extJwtIdx := -1
			totpIdx := -1

			for i, authQuery := range parsedBody.AuthQueries {
				if authQuery.TypeID == rest_model.AuthQueryTypeEXTDashJWT {
					extJwtIdx = i
				} else if authQuery.TypeID == rest_model.AuthQueryTypeTOTP {
					totpIdx = i
				} else {
					ctx.Req.Failf("unexexpected auth quuery type id encountered: %s", string(authQuery.TypeID))
				}
			}

			ctx.Req.True(extJwtIdx >= 0, "expected extJwtIdx to be set")
			ctx.Req.True(totpIdx >= 0, "expected totpIdx to be set")

			ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].ClientID, clientId)
			ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].Scopes[0], scope1)
			ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].Scopes[1], scope2)
			ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].HTTPURL, extAuthUrl)

			totpEnrollUrl := "https://" + ctx.ApiHost + "/oidc/login/totp/enroll"
			totpVerifyUrl := "https://" + ctx.ApiHost + "/oidc/login/totp/enroll/verify"

			t.Run("totp enroll flag is false", func(t *testing.T) {
				ctx.NextTest(t)

				ctx.False(parsedBody.AuthQueries[totpIdx].IsTotpEnrolled)
			})

			t.Run("can start totp enroll", func(t *testing.T) {
				ctx.NextTest(t)

				mfaDetail := &rest_model.DetailMfa{}
				resp, err = result.Client.R().SetHeader("content-type", "application/json").
					SetBody(map[string]string{"id": result.AuthRequestId}).
					SetResult(mfaDetail).
					Post(totpEnrollUrl)

				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
				ctx.Req.NotEmpty(mfaDetail.ID)
				ctx.Req.NotNil(mfaDetail.CreatedAt)
				ctx.Req.NotZero(mfaDetail.CreatedAt)
				ctx.Req.NotEmpty(mfaDetail.UpdatedAt)
				ctx.Req.NotZero(mfaDetail.UpdatedAt)
				ctx.Req.NotEmpty(mfaDetail.ProvisioningURL)
				ctx.Req.NotEmpty(mfaDetail.RecoveryCodes)

				t.Run("starting again errors", func(t *testing.T) {
					ctx.NextTest(t)
					apiError := &rest_model.APIError{}
					resp, err = result.Client.R().SetHeader("content-type", "application/json").
						SetBody(map[string]string{"id": result.AuthRequestId}).
						Post(totpEnrollUrl)

					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusConflict, resp.StatusCode())

					err = apiError.UnmarshalBinary(resp.Body())
					ctx.Req.NoError(err)
					ctx.Req.NotEmpty(apiError.Message)
					ctx.Req.NotEmpty(apiError.Code)
				})

				t.Run("deleting unverified MFA requires no code", func(t *testing.T) {
					ctx.NextTest(t)
					resp, err = result.Client.R().SetHeader("content-type", "application/json").
						SetBody(map[string]string{"id": result.AuthRequestId}).
						Delete(totpEnrollUrl)
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

					t.Run("after deleting can restart", func(t *testing.T) {
						ctx.NextTest(t)

						resp, err = result.Client.R().SetHeader("content-type", "application/json").
							SetBody(map[string]string{"id": result.AuthRequestId}).
							SetResult(mfaDetail).
							Post(totpEnrollUrl)

						ctx.Req.NoError(err)
						ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
						ctx.Req.NotEmpty(mfaDetail.ID)
						ctx.Req.NotNil(mfaDetail.CreatedAt)
						ctx.Req.NotZero(mfaDetail.CreatedAt)
						ctx.Req.NotEmpty(mfaDetail.UpdatedAt)
						ctx.Req.NotZero(mfaDetail.UpdatedAt)
						ctx.Req.NotEmpty(mfaDetail.ProvisioningURL)
						ctx.Req.NotEmpty(mfaDetail.RecoveryCodes)

						t.Run("verification fails with wrong code", func(t *testing.T) {
							ctx.NextTest(t)

							invalidCode := "123456"
							resp, err = result.Client.R().SetHeader("content-type", "application/json").
								SetBody(map[string]string{"id": result.AuthRequestId, "code": invalidCode}).
								Post(totpVerifyUrl)

							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
						})

						t.Run("verification fails with invalid characters", func(t *testing.T) {
							ctx.NextTest(t)

							invalidCode := "@#$^%$#%$&%$&#%&%$#"
							resp, err = result.Client.R().SetHeader("content-type", "application/json").
								SetBody(map[string]string{"id": result.AuthRequestId, "code": invalidCode}).
								Post(totpVerifyUrl)

							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
						})

						t.Run("verification fails with empty code", func(t *testing.T) {
							ctx.NextTest(t)

							invalidCode := ""
							resp, err = result.Client.R().SetHeader("content-type", "application/json").
								SetBody(map[string]string{"id": result.AuthRequestId, "code": invalidCode}).
								Post(totpVerifyUrl)

							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
						})

						t.Run("verification fails with a really short code", func(t *testing.T) {
							ctx.NextTest(t)

							invalidCode := "1"
							resp, err = result.Client.R().SetHeader("content-type", "application/json").
								SetBody(map[string]string{"id": result.AuthRequestId, "code": invalidCode}).
								Post(totpVerifyUrl)

							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
						})

						t.Run("verification fails with a really long code", func(t *testing.T) {
							ctx.NextTest(t)

							invalidCode := "430248509285928525809580250953850938520958032598058350913850585098103598103598135091385098109589358150913809s1"
							resp, err = result.Client.R().SetHeader("content-type", "application/json").
								SetBody(map[string]string{"id": result.AuthRequestId, "code": invalidCode}).
								Post(totpVerifyUrl)

							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
						})

						t.Run("verification fails with a recovery code", func(t *testing.T) {
							ctx.NextTest(t)

							invalidCode := mfaDetail.RecoveryCodes[0]
							resp, err = result.Client.R().SetHeader("content-type", "application/json").
								SetBody(map[string]string{"id": result.AuthRequestId, "code": invalidCode}).
								Post(totpVerifyUrl)

							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusBadRequest, resp.StatusCode())
						})

						t.Run("verification passes with correct code", func(t *testing.T) {
							ctx.NextTest(t)

							secret, err := parseSecretFromProvisioningUrl(mfaDetail.ProvisioningURL)
							ctx.Req.NoError(err)
							ctx.Req.NotEmpty(secret)

							validCode := computeMFACode(secret)

							resp, err = result.Client.R().SetHeader("content-type", "application/json").
								SetBody(map[string]string{"id": result.AuthRequestId, "code": validCode}).
								Post(totpVerifyUrl)

							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusOK, resp.StatusCode())
						})

						t.Run("reauthenticating shows that totp is enrolled", func(t *testing.T) {
							ctx.NextTest(t)

							reAuthResult, err := clientHelper.OidcAuthorize(identityCreds)
							ctx.Req.NoError(err)

							reAuthLoginUri := "https://" + ctx.ApiHost + "/oidc/login/username"

							resp, err := reAuthResult.Client.R().SetHeader("content-type", "application/json").
								SetBody(map[string]string{"id": reAuthResult.AuthRequestId, "username": identityName, "password": identityPassword}).
								Post(reAuthLoginUri)

							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusOK, resp.StatusCode())

							type respBody struct {
								AuthQueries []*rest_model.AuthQueryDetail `json:"authQueries"`
							}

							parsedBody := &respBody{}

							err = json.Unmarshal(resp.Body(), parsedBody)
							ctx.Req.NoError(err)

							ctx.Req.Len(parsedBody.AuthQueries, 2)

							extJwtIdx := -1
							totpIdx := -1

							for i, authQuery := range parsedBody.AuthQueries {
								if authQuery.TypeID == rest_model.AuthQueryTypeEXTDashJWT {
									extJwtIdx = i
								} else if authQuery.TypeID == rest_model.AuthQueryTypeTOTP {
									totpIdx = i
								} else {
									ctx.Req.Failf("unexexpected auth quuery type id encountered: %s", string(authQuery.TypeID))
								}
							}

							ctx.Req.True(extJwtIdx >= 0, "expected extJwtIdx to be set")
							ctx.Req.True(totpIdx >= 0, "expected totpIdx to be set")

							ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].ClientID, clientId)
							ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].Scopes[0], scope1)
							ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].Scopes[1], scope2)
							ctx.Req.Equal(parsedBody.AuthQueries[extJwtIdx].HTTPURL, extAuthUrl)

							t.Run("totp enroll flag is true", func(t *testing.T) {
								ctx.NextTest(t)

								ctx.True(parsedBody.AuthQueries[totpIdx].IsTotpEnrolled)
							})
						})
					})
				})
			})
		})

	})
}
