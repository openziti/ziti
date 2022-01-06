//go:build apitests
// +build apitests

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

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/dgryski/dgoogauth"
	"github.com/google/uuid"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/foundation/util/errorz"
	"image/png"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func Test_MFA(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	t.Run("if mfa is not enrolled", func(t *testing.T) {
		ctx.testContextChanged(t)

		noMfaIdentityId, noMfaIdentity := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment("mfa_ziti_test_no_mfa", false)
		noMfaSession, err := noMfaIdentity.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		t.Run("current identity", func(t *testing.T) {
			t.Run("get MFA should 404", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := noMfaSession.newAuthenticatedRequest().Get("/current-identity/mfa")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
			})

			t.Run("delete MFA should 404", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := noMfaSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Delete("/current-identity/mfa")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
			})

			t.Run("post MFA verify should 404", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := noMfaSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Post("/current-identity/mfa/verify")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
			})

			t.Run("get MFA QR code should 404", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := noMfaSession.newAuthenticatedRequest().Get("/current-identity/mfa/qr-code")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
			})

			t.Run("get MFA recovery codes should 404", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := noMfaSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Get("/current-identity/mfa/recovery-codes")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
			})

			t.Run("post MFA to generate recovery codes should 404", func(t *testing.T) {
				t.Run("get MFA recovery codes should 404", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := noMfaSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Post("/current-identity/mfa/recovery-codes")

					ctx.Req.NoError(err)
					standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
				})
			})

		})

		t.Run("current api session", func(t *testing.T) {
			t.Run("should not have authentication queries", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := noMfaSession.newAuthenticatedRequest().Get("/current-api-session")

				ctx.Req.NoError(err)
				standardJsonResponseTests(resp, http.StatusOK, t)

				currentApiSession, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.Req.True(currentApiSession.ExistsP("data.authQueries"), "authQueries exists on current api session")

				authQueriesVal := currentApiSession.Path("data.authQueries").Data()
				ctx.Req.NotNil(authQueriesVal, "authQueries should not be nil")

				authQueries := authQueriesVal.([]interface{})
				ctx.Req.NotNil(authQueries, "authQueries is an array")
				ctx.Req.Len(authQueries, 0, "authQueries has a length of 0")
			})
		})

		t.Run("admin identity endpoint", func(t *testing.T) {
			t.Run("get MFA should have isMfaEnabled flag set to false", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get(fmt.Sprintf("/identities/%s", noMfaIdentityId))

				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				identity, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.Req.True(identity.ExistsP("data.isMfaEnabled"))

				isMfaEnabledVal := identity.Path("data.isMfaEnabled").Data()
				ctx.Req.NotNil(isMfaEnabledVal, "isMfaEnabled flag is not nil")

				isMfaEnabled, ok := isMfaEnabledVal.(bool)
				ctx.Req.True(ok, "isMfaEnabled is a bool")
				ctx.Req.False(isMfaEnabled, "isMfaEnabled is false")
			})

			t.Run("delete MFA should 404", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete(fmt.Sprintf("/identities/%s/mfa", noMfaIdentityId))

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
			})
		})
	})

	t.Run("MFA enrollment can be initiated", func(t *testing.T) {
		ctx.testContextChanged(t)

		var mfaStartedIdentityId string
		var noMfaIdentity *certAuthenticator
		var mfaStartedSession *session
		mfaStartedIdentityName := "mfa_ziti_test_started"

		t.Run("by first authenticating", func(t *testing.T) {
			var err error
			mfaStartedIdentityId, noMfaIdentity = ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(mfaStartedIdentityName, false)
			mfaStartedSession, err = noMfaIdentity.AuthenticateClientApi(ctx)
			ctx.Req.NoError(err)
		})

		t.Run("by posting to /current-identity/mfa", func(t *testing.T) {
			var err error

			resp, err := mfaStartedSession.newAuthenticatedRequest().Post("/current-identity/mfa")
			ctx.Req.NoError(err)
			standardJsonResponseTests(resp, http.StatusCreated, t)
		})

		t.Run("and get MFA should return a MFA document", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mfaStartedSession.newAuthenticatedRequest().Get("/current-identity/mfa")
			ctx.Req.NoError(err)

			standardJsonResponseTests(resp, http.StatusOK, t)

			mfa, err := gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)
			ctx.Req.NotNil(mfa)

			t.Run("that is unverified", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.RequirePathExists(mfa, "data.isVerified")
				isVerified := ctx.RequireGetNonNilPathValue(mfa, "data.isVerified").Data().(bool)
				ctx.Req.False(isVerified, "data.isVerified not true")
			})

			t.Run("that has visible recovery codes", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.RequirePathExists(mfa, "data.recoveryCodes")
				interfaceArray := ctx.RequireGetNonNilPathValue(mfa, "data.recoveryCodes").Data().([]interface{})

				ctx.Req.Len(interfaceArray, 20, "has 20 recovery codes")

				for _, val := range interfaceArray {
					_, ok := val.(string)
					ctx.Req.True(ok, "all recovery code values should be strings")
				}
			})

			t.Run("that has valid provisioning URI", func(t *testing.T) {
				ctx.testContextChanged(t)

				ctx.RequirePathExists(mfa, "data.provisioningUrl")
				provisionString := ctx.RequireGetNonNilPathValue(mfa, "data.provisioningUrl").Data().(string)
				ctx.Req.NotEmpty(provisionString)

				mfaUrl, err := url.Parse(provisionString)
				ctx.Req.NoError(err)
				ctx.Req.Equal(mfaUrl.Host, "totp")
				ctx.Req.Equal(mfaUrl.Path, "/ziti.dev:"+mfaStartedIdentityName)
				ctx.Req.Equal(mfaUrl.Scheme, "otpauth")
			})
		})

		t.Run("and for the admin identity endpoint", func(t *testing.T) {
			t.Run("get MFA should have isMfaEnabled flag set to false", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get(fmt.Sprintf("/identities/%s", mfaStartedIdentityId))

				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				identity, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.Req.True(identity.ExistsP("data.isMfaEnabled"))

				isMfaEnabledVal := identity.Path("data.isMfaEnabled").Data()
				ctx.Req.NotNil(isMfaEnabledVal, "isMfaEnabled flag is not nil")

				isMfaEnabled, ok := isMfaEnabledVal.(bool)
				ctx.Req.True(ok, "isMfaEnabled is a bool")
				ctx.Req.False(isMfaEnabled, "isMfaEnabled is false")
			})

			t.Run("delete MFA should fail with 404", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete(fmt.Sprintf("/identities/%s/mfa", mfaStartedIdentityId))

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
			})
		})

		t.Run("and get MFA QR code should return a PNG", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := mfaStartedSession.newAuthenticatedRequest().Get("/current-identity/mfa/qr-code")
			ctx.Req.NoError(err)
			ctx.Req.Equal(resp.StatusCode(), http.StatusOK)
			ctx.Req.Equal(resp.Header().Get("content-type"), "image/png")

			img, err := png.Decode(bytes.NewReader(resp.Body()))
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(img)
		})

		t.Run("and get MFA recovery codes should return 404", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mfaStartedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Get("/current-identity/mfa/recovery-codes")
			ctx.Req.NoError(err)

			standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
		})

		t.Run("and post MFA recovery codes should error with not found", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mfaStartedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Post("/current-identity/mfa/recovery-codes")
			ctx.Req.NoError(err)

			standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
		})

		t.Run("authentication with partially enrolled MFA should fully authenticate", func(t *testing.T) {
			ctx.testContextChanged(t)

			secondMfaStartedSession, err := noMfaIdentity.AuthenticateClientApi(ctx)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(secondMfaStartedSession)

			resp, err := secondMfaStartedSession.newAuthenticatedRequest().Get("/current-api-session/")
			standardJsonResponseTests(resp, http.StatusOK, t)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)

			currentSession, err := gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)

			t.Run("and not have any auth queries", func(s *testing.T) {
				ctx.testContextChanged(t)
				ctx.RequirePathExists(currentSession, "data.authQueries")
				authQueriesArray := ctx.RequireGetNonNilPathValue(currentSession, "data.authQueries").Data().([]interface{})
				ctx.Req.Len(authQueriesArray, 0)
			})

			t.Run("and have full API access", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := secondMfaStartedSession.newAuthenticatedRequest().Get("/sessions")
				ctx.Req.NoError(err)
				standardJsonResponseTests(resp, http.StatusOK, t)
			})
		})

		t.Run("and delete MFA should remove with blank code", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mfaStartedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Delete("/current-identity/mfa")
			ctx.Req.NoError(err)

			standardJsonResponseTests(resp, http.StatusOK, t)

			t.Run("and return 404 if requested after deletion", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := mfaStartedSession.newAuthenticatedRequest().Get("/current-identity/mfa")
				ctx.Req.NoError(err)

				standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
			})
		})
	})

	t.Run("if MFA verification is attempted", func(t *testing.T) {
		ctx.testContextChanged(t)

		var mfaValidatedIdentityId string
		var mfaValidatedIdentity *certAuthenticator
		var mfaValidatedSession *session
		mfaStartedIdentityName := "mfa_ziti_test_validated"

		var mfaValidatedRecoveryCodes []string

		var mfaValidatedSecret string

		t.Run("setup", func(t *testing.T) {
			t.Run("by first authenticating", func(t *testing.T) {
				var err error
				mfaValidatedIdentityId, mfaValidatedIdentity = ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(mfaStartedIdentityName, false)
				mfaValidatedSession, err = mfaValidatedIdentity.AuthenticateClientApi(ctx)
				ctx.Req.NotEmpty(mfaValidatedIdentityId)
				ctx.Req.NoError(err)
			})

			t.Run("by posting to /current-identity/mfa", func(t *testing.T) {
				var err error

				resp, err := mfaValidatedSession.newAuthenticatedRequest().Post("/current-identity/mfa")
				ctx.Req.NoError(err)
				standardJsonResponseTests(resp, http.StatusCreated, t)
			})

			t.Run("save recovery codes", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := mfaValidatedSession.newAuthenticatedRequest().Get("/current-identity/mfa")
				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				mfa, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.RequirePathExists(mfa, "data.recoveryCodes")
				interfaceArray := ctx.RequireGetNonNilPathValue(mfa, "data.recoveryCodes").Data().([]interface{})

				ctx.Req.Len(interfaceArray, 20, "has 20 recovery codes")

				for _, val := range interfaceArray {
					recoveryCode, ok := val.(string)
					ctx.Req.True(ok, "all recovery code values should be strings")
					mfaValidatedRecoveryCodes = append(mfaValidatedRecoveryCodes, recoveryCode)
				}

				t.Run("prepping otp code generation", func(t *testing.T) {
					ctx.testContextChanged(t)

					ctx.RequirePathExists(mfa, "data.provisioningUrl")
					provisionString := ctx.RequireGetNonNilPathValue(mfa, "data.provisioningUrl").Data().(string)

					parsedUrl, err := url.Parse(provisionString)
					ctx.Req.NoError(err)
					ctx.Req.Equal(parsedUrl.Host, "totp")

					mfaValidatedSecret = provisionString

					queryParams, err := url.ParseQuery(parsedUrl.RawQuery)
					ctx.Req.NoError(err)
					secrets := queryParams["secret"]
					ctx.Req.NotNil(secrets)
					ctx.Req.NotEmpty(secrets)

					mfaValidatedSecret = secrets[0]
				})
			})
		})

		t.Run("with a missing code it should error with invalid token/bad request", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(`{}`).Post("/current-identity/mfa/verify")

			ctx.Req.NoError(err)
			standardErrorJsonResponseTests(resp, "COULD_NOT_VALIDATE", http.StatusBadRequest, t)
		})

		t.Run("with an empty code it should error with invalid token/bad request", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Post("/current-identity/mfa/verify")

			ctx.Req.NoError(err)
			standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
		})

		t.Run("with an invalid code it should error with invalid token/bad request", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.testContextChanged(t)
			resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("qwgjwoigrw92")).Post("/current-identity/mfa/verify")

			ctx.Req.NoError(err)
			standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
		})

		t.Run("verify with a recovery code should return with invalid token/bad request", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(mfaValidatedRecoveryCodes[2])).Post("/current-identity/mfa/verify")

			ctx.Req.NoError(err)
			standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
		})

		t.Run("with a valid code should return ok", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.Req.NotEmpty(mfaValidatedSecret)
			code := computeMFACode(mfaValidatedSecret)

			resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(code)).Post("/current-identity/mfa/verify")

			ctx.Req.NoError(err)
			standardJsonResponseTests(resp, http.StatusOK, t)

			t.Run("current api session should be", func(t *testing.T) {
				ctx.testContextChanged(t)
				respContainer := mfaValidatedSession.requireQuery("/current-api-session")

				t.Run("marked as mfa required", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.Req.True(respContainer.ExistsP("data.isMfaRequired"), "could not find isMfaRequired by path")
					isMfaRequired, ok := respContainer.Path("data.isMfaRequired").Data().(bool)
					ctx.Req.True(ok, "should be a bool")
					ctx.Req.True(isMfaRequired)
				})

				t.Run("marked as mfa complete", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.Req.True(respContainer.ExistsP("data.isMfaComplete"), "could not find isMfaComplete by path")
					isMfaComplete, ok := respContainer.Path("data.isMfaComplete").Data().(bool)
					ctx.Req.True(ok, "should be a bool")
					ctx.Req.True(isMfaComplete)
				})
			})

			t.Run("api session should have posture data with MFA passed", func(t *testing.T) {
				ctx.testContextChanged(t)
				postureDataContainer := ctx.AdminManagementSession.requireQuery(fmt.Sprintf("/identities/%s/posture-data", mfaValidatedIdentityId))
				mfaPath := fmt.Sprintf("data.apiSessionPostureData.%s.mfa.passedMfa", mfaValidatedSession.id)
				postureDataContainer.ExistsP(mfaPath)
				isMfaPassed, ok := postureDataContainer.Path(mfaPath).Data().(bool)
				ctx.Req.True(ok, "should be a bool")
				ctx.Req.True(isMfaPassed)
			})
		})

		t.Run("a second verify with should error with not found", func(t *testing.T) {
			ctx.testContextChanged(t)

			ctx.Req.NotEmpty(mfaValidatedSecret)
			code := computeMFACode(mfaValidatedSecret)

			resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(code)).Post("/current-identity/mfa/verify")

			ctx.Req.NoError(err)
			standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
		})

		t.Run("after verification", func(t *testing.T) {
			t.Run("get MFA qr code should error with not found", func(s *testing.T) {
				ctx.testContextChanged(t)
				resp, err := mfaValidatedSession.newAuthenticatedRequest().Get("/current-identity/mfa/qr-code")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, errorz.NotFoundCode, http.StatusNotFound, t)
			})

			t.Run("get MFA should return a MFA documentt", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := mfaValidatedSession.newAuthenticatedRequest().Get("/current-identity/mfa")
				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				mfa, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				t.Run("that is verified", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.RequirePathExists(mfa, "data.isVerified")
					isVerified, ok := ctx.RequireGetNonNilPathValue(mfa, "data.isVerified").Data().(bool)

					ctx.Req.True(ok)
					ctx.Req.True(isVerified)
				})

				t.Run("that does not have recovery codes", func(t *testing.T) {
					ctx.testContextChanged(t)
					recoveryCodes := mfa.Path("data.recoveryCodes").Data()
					ctx.Req.Nil(recoveryCodes)
				})

				t.Run("that does not have a provisioning URI", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.Req.False(mfa.ExistsP("data.provisioningUrl"))
				})
			})

			t.Run("get recovery codes with empty code should return invalid token/bad request", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Get("/current-identity/mfa/recovery-codes")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
			})

			t.Run("get recovery codes with an invalid code should return invalid token/bad request", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.testContextChanged(t)
				resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("6sa5d4f56sad")).Get("/current-identity/mfa/recovery-codes")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
			})

			t.Run("get recovery codes with a recovery code should return invalid token/bad request", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.testContextChanged(t)
				resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(mfaValidatedRecoveryCodes[10])).Get("/current-identity/mfa/recovery-codes")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
			})

			t.Run("create new recovery codes with empty code should return invalid token/bad request", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Post("/current-identity/mfa/recovery-codes")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
			})

			t.Run("create recovery codes with an invalid code should return invalid token/bad request", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.testContextChanged(t)
				resp, err := mfaValidatedSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("6sa5d4f56sad")).Post("/current-identity/mfa/recovery-codes")

				ctx.Req.NoError(err)
				standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
			})

			t.Run("newly authenticated sessions can use TOTP codes for authentication", func(t *testing.T) {
				var newValidatedSession *session

				t.Run("by first authenticating", func(t *testing.T) {
					var err error
					newValidatedSession, err = mfaValidatedIdentity.AuthenticateClientApi(ctx)
					ctx.Req.NoError(err)
				})

				t.Run("should have one authentication query", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := newValidatedSession.newAuthenticatedRequest().Get("/current-api-session")

					ctx.Req.NoError(err)
					standardJsonResponseTests(resp, http.StatusOK, t)

					currentApiSession, err := gabs.ParseJSON(resp.Body())
					ctx.Req.NoError(err)

					ctx.Req.True(currentApiSession.ExistsP("data.authQueries"), "authQueries exists on current api session")

					authQueriesVal := currentApiSession.Path("data.authQueries").Data()
					ctx.Req.NotNil(authQueriesVal, "authQueries should not be nil")

					authQueries := authQueriesVal.([]interface{})
					ctx.Req.NotNil(authQueries, "authQueries is an array")
					ctx.Req.Len(authQueries, 1, "authQueries has a length of 1")

					t.Run("should be a Ziti MFA authentication query", func(t *testing.T) {
						ctx.testContextChanged(t)

						authQuery, ok := authQueries[0].(map[string]interface{})

						ctx.Req.True(ok)
						ctx.Req.NotEmpty(authQuery)

						ctx.Req.Equal("alphaNumeric", authQuery["format"].(string))
						ctx.Req.Equal("POST", authQuery["httpMethod"].(string))
						ctx.Req.Equal("./authenticate/mfa", authQuery["httpUrl"].(string))
						ctx.Req.Equal(float64(4), authQuery["minLength"].(float64))
						ctx.Req.Equal(float64(6), authQuery["maxLength"].(float64))
						ctx.Req.Equal("ziti", authQuery["provider"].(string))
					})
				})

				t.Run("should not have access to the API with outstanding authQuery", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := newValidatedSession.newAuthenticatedRequest().Get("/sessions")
					ctx.Req.NoError(err)

					standardErrorJsonResponseTests(resp, errorz.UnauthorizedCode, http.StatusUnauthorized, t)
				})

				t.Run("validating MFA with an empty token should fail with invalid token/bad request", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := newValidatedSession.newAuthenticatedRequest().
						SetBody(newMfaCodeBody("")).
						Post("/authenticate/mfa")

					ctx.Req.NoError(err)

					standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
				})

				t.Run("validating MFA with an invalid token should fail with invalid token/bad request", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := newValidatedSession.newAuthenticatedRequest().
						SetBody(newMfaCodeBody("8f4b48er")).
						Post("/authenticate/mfa")

					ctx.Req.NoError(err)

					standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
				})

				t.Run("validating MFA with an valid token should succeed", func(t *testing.T) {
					ctx.testContextChanged(t)

					code := computeMFACode(mfaValidatedSecret)

					resp, err := newValidatedSession.newAuthenticatedRequest().
						SetBody(newMfaCodeBody(code)).
						Post("/authenticate/mfa")

					ctx.Req.NoError(err)

					standardJsonResponseTests(resp, http.StatusOK, t)

					t.Run("and should have access to the API", func(t *testing.T) {
						ctx.testContextChanged(t)

						resp, err := newValidatedSession.newAuthenticatedRequest().Get("/sessions")
						ctx.Req.NoError(err)

						standardJsonResponseTests(resp, http.StatusOK, t)
					})

					t.Run("current api session should be", func(t *testing.T) {
						ctx.testContextChanged(t)
						respContainer := newValidatedSession.requireQuery("/current-api-session")

						t.Run("marked as mfa required", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.True(respContainer.ExistsP("data.isMfaRequired"), "could not find isMfaRequired by path")
							isMfaRequired, ok := respContainer.Path("data.isMfaRequired").Data().(bool)
							ctx.Req.True(ok, "should be a bool")
							ctx.Req.True(isMfaRequired)
						})

						t.Run("marked as mfa complete", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.True(respContainer.ExistsP("data.isMfaComplete"), "could not find isMfaComplete by path")
							isMfaComplete, ok := respContainer.Path("data.isMfaComplete").Data().(bool)
							ctx.Req.True(ok, "should be a bool")
							ctx.Req.True(isMfaComplete)
						})
					})

					t.Run("api session should have posture data with MFA passed", func(t *testing.T) {
						ctx.testContextChanged(t)
						postureDataContainer := ctx.AdminManagementSession.requireQuery(fmt.Sprintf("/identities/%s/posture-data", newValidatedSession.identityId))
						mfaPath := fmt.Sprintf("data.apiSessionPostureData.%s.mfa.passedMfa", newValidatedSession.id)
						postureDataContainer.ExistsP(mfaPath)
						isMfaPassed, ok := postureDataContainer.Path(mfaPath).Data().(bool)
						ctx.Req.True(ok, "should be a bool")
						ctx.Req.True(isMfaPassed)
					})
				})

			})

			t.Run("newly authenticated sessions can use recovery codes for authentication", func(t *testing.T) {
				var newValidatedSession *session

				t.Run("by first authenticating", func(t *testing.T) {
					var err error
					newValidatedSession, err = mfaValidatedIdentity.AuthenticateClientApi(ctx)
					ctx.Req.NoError(err)
				})

				t.Run("should have one authentication query", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := newValidatedSession.newAuthenticatedRequest().Get("/current-api-session")

					ctx.Req.NoError(err)
					standardJsonResponseTests(resp, http.StatusOK, t)

					currentApiSession, err := gabs.ParseJSON(resp.Body())
					ctx.Req.NoError(err)

					ctx.Req.True(currentApiSession.ExistsP("data.authQueries"), "authQueries exists on current api session")

					authQueriesVal := currentApiSession.Path("data.authQueries").Data()
					ctx.Req.NotNil(authQueriesVal, "authQueries should not be nil")

					authQueries := authQueriesVal.([]interface{})
					ctx.Req.NotNil(authQueries, "authQueries is an array")
					ctx.Req.Len(authQueries, 1, "authQueries has a length of 1")

					t.Run("should be a Ziti MFA authentication query", func(t *testing.T) {
						ctx.testContextChanged(t)

						authQuery, ok := authQueries[0].(map[string]interface{})

						ctx.Req.True(ok)
						ctx.Req.NotEmpty(authQuery)

						ctx.Req.Equal("alphaNumeric", authQuery["format"].(string))
						ctx.Req.Equal("POST", authQuery["httpMethod"].(string))
						ctx.Req.Equal("./authenticate/mfa", authQuery["httpUrl"].(string))
						ctx.Req.Equal(float64(4), authQuery["minLength"].(float64))
						ctx.Req.Equal(float64(6), authQuery["maxLength"].(float64))
						ctx.Req.Equal("ziti", authQuery["provider"].(string))
					})
				})

				t.Run("should not have access to the API with outstanding authQuery", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := newValidatedSession.newAuthenticatedRequest().Get("/sessions")
					ctx.Req.NoError(err)

					standardErrorJsonResponseTests(resp, errorz.UnauthorizedCode, http.StatusUnauthorized, t)
				})

				t.Run("validating MFA with an valid recovery code should succeed", func(t *testing.T) {
					ctx.testContextChanged(t)

					code := mfaValidatedRecoveryCodes[0]

					//remove code to be used
					mfaValidatedRecoveryCodes = mfaValidatedRecoveryCodes[1:]

					resp, err := newValidatedSession.newAuthenticatedRequest().
						SetBody(newMfaCodeBody(code)).
						Post("/authenticate/mfa")

					ctx.Req.NoError(err)

					standardJsonResponseTests(resp, http.StatusOK, t)

					t.Run("and should have access to the API", func(t *testing.T) {
						ctx.testContextChanged(t)

						resp, err := newValidatedSession.newAuthenticatedRequest().Get("/sessions")
						ctx.Req.NoError(err)

						standardJsonResponseTests(resp, http.StatusOK, t)
					})

					t.Run("current api session should be", func(t *testing.T) {
						ctx.testContextChanged(t)
						respContainer := newValidatedSession.requireQuery("/current-api-session")

						t.Run("marked as mfa required", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.True(respContainer.ExistsP("data.isMfaRequired"), "could not find isMfaRequired by path")
							isMfaRequired, ok := respContainer.Path("data.isMfaRequired").Data().(bool)
							ctx.Req.True(ok, "should be a bool")
							ctx.Req.True(isMfaRequired)
						})

						t.Run("marked as mfa complete", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.True(respContainer.ExistsP("data.isMfaComplete"), "could not find isMfaComplete by path")
							isMfaComplete, ok := respContainer.Path("data.isMfaComplete").Data().(bool)
							ctx.Req.True(ok, "should be a bool")
							ctx.Req.True(isMfaComplete)
						})
					})

					t.Run("api session should have posture data with MFA passed", func(t *testing.T) {
						ctx.testContextChanged(t)
						postureDataContainer := ctx.AdminManagementSession.requireQuery(fmt.Sprintf("/identities/%s/posture-data", newValidatedSession.identityId))
						mfaPath := fmt.Sprintf("data.apiSessionPostureData.%s.mfa.passedMfa", newValidatedSession.id)
						postureDataContainer.ExistsP(mfaPath)
						isMfaPassed, ok := postureDataContainer.Path(mfaPath).Data().(bool)
						ctx.Req.True(ok, "should be a bool")
						ctx.Req.True(isMfaPassed)
					})
				})

			})

			t.Run("admin identity endpoint", func(t *testing.T) {
				t.Run("get MFA should have isMfaEnabled flag set to true", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get(fmt.Sprintf("/identities/%s", mfaValidatedIdentityId))

					ctx.Req.NoError(err)

					standardJsonResponseTests(resp, http.StatusOK, t)

					identity, err := gabs.ParseJSON(resp.Body())
					ctx.Req.NoError(err)

					ctx.Req.True(identity.ExistsP("data.isMfaEnabled"))

					isMfaEnabledVal := identity.Path("data.isMfaEnabled").Data()
					ctx.Req.NotNil(isMfaEnabledVal, "isMfaEnabled flag is not nil")

					isMfaEnabled, ok := isMfaEnabledVal.(bool)
					ctx.Req.True(ok, "isMfaEnabled is a bool")
					ctx.Req.True(isMfaEnabled, "isMfaEnabled is true")
				})
			})
		})
	})

	t.Run("if MFA enrollment is removed by an admin", func(t *testing.T) {

		ctx.testContextChanged(t)

		var mfa01DeleteIdentityId string
		var mfa01Delete *certAuthenticator
		var mfa01DeleteSession *session
		mfa01DeleteName := "mfa_ziti_test_delete01"
		var mfa01DeleteRecoveryCodes []string
		var mfa01DeleteSecret string

		t.Run("setup", func(t *testing.T) {
			t.Run("by first authenticating", func(t *testing.T) {
				var err error
				mfa01DeleteIdentityId, mfa01Delete = ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(mfa01DeleteName, false)
				mfa01DeleteSession, err = mfa01Delete.AuthenticateClientApi(ctx)
				ctx.Req.NotEmpty(mfa01DeleteIdentityId)
				ctx.Req.NoError(err)
			})

			t.Run("by posting to /current-identity/mfa", func(t *testing.T) {
				var err error

				resp, err := mfa01DeleteSession.newAuthenticatedRequest().Post("/current-identity/mfa")
				ctx.Req.NoError(err)
				standardJsonResponseTests(resp, http.StatusCreated, t)
			})

			t.Run("save recovery codes", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := mfa01DeleteSession.newAuthenticatedRequest().Get("/current-identity/mfa")
				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				mfa, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.RequirePathExists(mfa, "data.recoveryCodes")
				interfaceArray := ctx.RequireGetNonNilPathValue(mfa, "data.recoveryCodes").Data().([]interface{})

				ctx.Req.Len(interfaceArray, 20, "has 20 recovery codes")

				for _, val := range interfaceArray {
					recoveryCode, ok := val.(string)
					ctx.Req.True(ok, "all recovery code values should be strings")
					mfa01DeleteRecoveryCodes = append(mfa01DeleteRecoveryCodes, recoveryCode)
				}

				t.Run("prepping otp code generation", func(t *testing.T) {
					ctx.testContextChanged(t)

					ctx.RequirePathExists(mfa, "data.provisioningUrl")
					provisionString := ctx.RequireGetNonNilPathValue(mfa, "data.provisioningUrl").Data().(string)

					parsedUrl, err := url.Parse(provisionString)
					ctx.Req.NoError(err)
					ctx.Req.Equal(parsedUrl.Host, "totp")

					mfa01DeleteSecret = provisionString

					queryParams, err := url.ParseQuery(parsedUrl.RawQuery)
					ctx.Req.NoError(err)
					secrets := queryParams["secret"]
					ctx.Req.NotNil(secrets)
					ctx.Req.NotEmpty(secrets)

					mfa01DeleteSecret = secrets[0]
				})

				t.Run("verifying MFA", func(t *testing.T) {
					ctx.testContextChanged(t)

					code := computeMFACode(mfa01DeleteSecret)

					resp, err := mfa01DeleteSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(code)).Post("/current-identity/mfa/verify")

					ctx.Req.NoError(err)
					standardJsonResponseTests(resp, http.StatusOK, t)
				})
			})
		})

		t.Run("by a non-admin it should fail", func(t *testing.T) {
			ctx.testContextChanged(t)

			_, nonAdmin := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(uuid.New().String(), false)
			nonAdminSession, err := nonAdmin.AuthenticateManagementApi(ctx)

			ctx.Req.NoError(err)

			resp, err := nonAdminSession.newAuthenticatedRequest().Delete("/identities/" + mfa01DeleteIdentityId + "/mfa")
			ctx.Req.NoError(err)

			standardErrorJsonResponseTests(resp, errorz.UnauthorizedCode, http.StatusUnauthorized, t)
		})

		t.Run("by an admin it should succeed", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Delete("/identities/" + mfa01DeleteIdentityId + "/mfa")
			ctx.Req.NoError(err)

			standardJsonResponseTests(resp, http.StatusOK, t)

			t.Run("mfaEnabled flag should be false", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get(fmt.Sprintf("/identities/%s", mfa01DeleteIdentityId))

				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				identity, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.Req.True(identity.ExistsP("data.isMfaEnabled"))

				isMfaEnabledVal := identity.Path("data.isMfaEnabled").Data()
				ctx.Req.NotNil(isMfaEnabledVal, "isMfaEnabled flag is not nil")

				isMfaEnabled, ok := isMfaEnabledVal.(bool)
				ctx.Req.True(ok, "isMfaEnabled is a bool")
				ctx.Req.False(isMfaEnabled, "isMfaEnabled is false")
			})
		})
	})

	t.Run("if mfa is removed by user", func(t *testing.T) {
		var mfa02DeleteIdentityId string
		var mfa02Delete *certAuthenticator
		var mfa02DeleteSession *session
		mfa02DeleteName := "mfa_ziti_test_delete02"
		var mfa02DeleteRecoveryCodes []string
		var mfa02DeleteSecret string

		t.Run("setup", func(t *testing.T) {
			t.Run("by first authenticating", func(t *testing.T) {
				var err error
				mfa02DeleteIdentityId, mfa02Delete = ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(mfa02DeleteName, false)
				mfa02DeleteSession, err = mfa02Delete.AuthenticateClientApi(ctx)
				ctx.Req.NotEmpty(mfa02DeleteIdentityId)
				ctx.Req.NoError(err)
			})

			t.Run("by posting to /current-identity/mfa", func(t *testing.T) {
				var err error

				resp, err := mfa02DeleteSession.newAuthenticatedRequest().Post("/current-identity/mfa")
				ctx.Req.NoError(err)
				standardJsonResponseTests(resp, http.StatusCreated, t)
			})

			t.Run("save recovery codes", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := mfa02DeleteSession.newAuthenticatedRequest().Get("/current-identity/mfa")
				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				mfa, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.RequirePathExists(mfa, "data.recoveryCodes")
				interfaceArray := ctx.RequireGetNonNilPathValue(mfa, "data.recoveryCodes").Data().([]interface{})

				ctx.Req.Len(interfaceArray, 20, "has 20 recovery codes")

				for _, val := range interfaceArray {
					recoveryCode, ok := val.(string)
					ctx.Req.True(ok, "all recovery code values should be strings")
					mfa02DeleteRecoveryCodes = append(mfa02DeleteRecoveryCodes, recoveryCode)
				}

				t.Run("prepping otp code generation", func(t *testing.T) {
					ctx.testContextChanged(t)

					ctx.RequirePathExists(mfa, "data.provisioningUrl")
					provisionString := ctx.RequireGetNonNilPathValue(mfa, "data.provisioningUrl").Data().(string)

					parsedUrl, err := url.Parse(provisionString)
					ctx.Req.NoError(err)
					ctx.Req.Equal(parsedUrl.Host, "totp")

					mfa02DeleteSecret = provisionString

					queryParams, err := url.ParseQuery(parsedUrl.RawQuery)
					ctx.Req.NoError(err)
					secrets := queryParams["secret"]
					ctx.Req.NotNil(secrets)
					ctx.Req.NotEmpty(secrets)

					mfa02DeleteSecret = secrets[0]
				})

				t.Run("verifying MFA", func(t *testing.T) {
					ctx.testContextChanged(t)

					code := computeMFACode(mfa02DeleteSecret)

					resp, err := mfa02DeleteSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(code)).Post("/current-identity/mfa/verify")

					ctx.Req.NoError(err)
					standardJsonResponseTests(resp, http.StatusOK, t)
				})
			})
		})

		t.Run("with an empty code it should fail", func(s *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mfa02DeleteSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("")).Delete("/current-identity/mfa")

			ctx.Req.NoError(err)
			standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
		})

		t.Run("with an invalid code it should fail", func(s *testing.T) {
			ctx.testContextChanged(t)
			resp, err := mfa02DeleteSession.newAuthenticatedRequest().SetBody(newMfaCodeBody("456456465")).Delete("/current-identity/mfa")

			ctx.Req.NoError(err)
			standardErrorJsonResponseTests(resp, apierror.MfaInvalidTokenCode, http.StatusBadRequest, t)
		})

		t.Run("with a valid code it should succeed", func(s *testing.T) {
			ctx.testContextChanged(t)

			code := computeMFACode(mfa02DeleteSecret)

			resp, err := mfa02DeleteSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(code)).Delete("/current-identity/mfa")

			ctx.Req.NoError(err)
			standardJsonResponseTests(resp, http.StatusOK, t)
		})
	})
}

func computeMFACode(secret string) string {
	now := int64(time.Now().UTC().Unix() / 30)
	code := dgoogauth.ComputeCode(secret, now)

	//pad leading 0s to 6 characters
	return fmt.Sprintf("%06d", code)
}

type mfaCode struct {
	Code string `json:"code"`
}

func newMfaCodeBody(code string) []byte {
	val := mfaCode{
		Code: code,
	}
	body, _ := json.Marshal(val)
	return body
}
