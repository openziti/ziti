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
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/edge/eid"
	"net/http"
	"net/url"
	"testing"
)

func Test_PostureChecks_MFA(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	t.Run("can create a MFA posture check associated to a service", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckMFA(s(postureCheckRole))

		ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))

		ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))

		ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

		ctx.Req.NoError(err)

		t.Run("identity can see service via policies", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(enrolledIdentitySession.isServiceVisibleToUser(service.Id))
		})

		t.Run("service has the posture check in its queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			code, body := enrolledIdentitySession.query("/services/" + service.Id)
			ctx.Req.Equal(http.StatusOK, code)
			entityService, err := gabs.ParseJSON(body)
			ctx.Req.NoError(err)

			querySet, err := entityService.Path("data.postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(querySet, 1)

			postureQueries, err := querySet[0].Path("postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(postureQueries, 1)

			ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
			ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
				ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
			})
		})

		t.Run("cannot create session with failing queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
		})

		t.Run("providing valid posture data", func(t *testing.T) {
			ctx.testContextChanged(t)

			mfaSecret := ""

			t.Run("by starting enrollment in MFA", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := enrolledIdentitySession.newAuthenticatedRequest().Post("/current-identity/mfa")

				ctx.Req.NoError(err)
				standardJsonResponseTests(resp, http.StatusCreated, t)

				t.Run("does not allow service session creation", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := enrolledIdentitySession.createNewSession(service.Id)
					ctx.Req.NoError(err)

					ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
				})
			})

			t.Run("by finishing enrollment in MFA", func(t *testing.T) {
				resp, err := enrolledIdentitySession.newAuthenticatedRequest().Get("/current-identity/mfa")
				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				mfa, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				rawUrl := mfa.Path("data.provisioningUrl").Data().(string)
				ctx.Req.NotEmpty(rawUrl)

				parsedUrl, err := url.Parse(rawUrl)
				ctx.Req.NoError(err)

				queryParams, err := url.ParseQuery(parsedUrl.RawQuery)
				ctx.Req.NoError(err)
				secrets := queryParams["secret"]
				ctx.Req.NotNil(secrets)
				ctx.Req.NotEmpty(secrets)

				mfaSecret = secrets[0]

				ctx.testContextChanged(t)

				code := computeMFACode(mfaSecret)

				resp, err = enrolledIdentitySession.newAuthenticatedRequest().
					SetBody(newMfaCodeBody(code)).
					Post("/current-identity/mfa/verify")

				ctx.Req.NoError(err)
				standardJsonResponseTests(resp, http.StatusOK, t)

				t.Run("allows service session creation", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := enrolledIdentitySession.createNewSession(service.Id)
					ctx.Req.NoError(err)

					ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
				})
			})

			t.Run("a new api session can pass MFA auth to create service session", func(t *testing.T) {
				ctx.testContextChanged(t)

				newSession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)
				ctx.Req.NoError(err)

				resp, err := newSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(computeMFACode(mfaSecret))).Post("/authenticate/mfa")
				ctx.Req.NoError(err)

				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				resp, err = newSession.createNewSession(service.Id)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

				sessionContainer, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.Req.True(sessionContainer.ExistsP("data.id"))
				sessionId, ok := sessionContainer.Path("data.id").Data().(string)
				ctx.Req.True(ok)
				ctx.Req.NotEmpty(sessionId)

				t.Run("removing MFA", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := newSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(computeMFACode(mfaSecret))).Delete("/current-identity/mfa")
					ctx.Req.NoError(err)

					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

					t.Run("removes existing session", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := newSession.newAuthenticatedRequest().Get(fmt.Sprintf("/sessions/%s", sessionId))
						ctx.Req.NoError(err)
						ctx.Req.Equal(http.StatusNotFound, resp.StatusCode())
					})

					t.Run("doesn't allow new sessions", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := enrolledIdentitySession.createNewSession(service.Id)
						ctx.Req.NoError(err)

						ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
					})
				})
			})
		})
	})
}
