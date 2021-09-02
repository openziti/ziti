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
	"github.com/google/uuid"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/rest_model"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func Test_PostureChecks_MFA(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	t.Run("can create a MFA posture check associated to a service with no prompts and no timeout", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		origTimeout := int64(-1)
		postureCheck := rest_model.PostureCheckMfaCreate{
			PostureCheckMfaProperties: rest_model.PostureCheckMfaProperties{
				PromptOnUnlock: false,
				PromptOnWake:   false,
				TimeoutSeconds: origTimeout,
			},
		}

		origName := uuid.New().String()
		postureCheck.SetName(&origName)
		postureCheck.SetTypeID(rest_model.PostureCheckTypeMFA)

		origAttributes := rest_model.Attributes(s(postureCheckRole))
		postureCheck.SetRoleAttributes(&origAttributes)
		postureCheckJson, err := postureCheck.MarshalJSON()
		ctx.Req.NoError(err)
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(postureCheckJson).Post("/posture-checks")
		ctx.Req.NoError(err)
		standardJsonResponseTests(resp, http.StatusCreated, t)

		postureCheckId := ctx.getEntityId(resp.Body())
		ctx.Req.NotEmpty(postureCheckId)

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

			ctx.Req.Equal(postureCheckId, postureQueries[0].Path("id").Data().(string))
			ctx.Req.Equal(string(postureCheck.TypeID()), postureQueries[0].Path("queryType").Data().(string))

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
				ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
				ctx.Req.Equal(-1, int(postureQueries[0].Path("timeout").Data().(float64)))
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

					ctx.Req.Equal(postureCheckId, postureQueries[0].Path("id").Data().(string))
					ctx.Req.Equal(string(postureCheck.TypeID()), postureQueries[0].Path("queryType").Data().(string))

					t.Run("query is currently passing", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
						ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
						ctx.Req.Equal(-1, int(postureQueries[0].Path("timeout").Data().(float64)))
					})
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

				t.Run("removing MFA while the service requires MFA", func(t *testing.T) {
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

	t.Run("can create a MFA posture check with promptOnWake, promptOnUnlock associated to a service", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))

		ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))

		ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

		origTimeout := int64(5000)
		postureCheck := rest_model.PostureCheckMfaCreate{
			PostureCheckMfaProperties: rest_model.PostureCheckMfaProperties{
				PromptOnUnlock: true,
				PromptOnWake:   true,
				TimeoutSeconds: origTimeout,
			},
		}

		origName := uuid.New().String()
		postureCheck.SetName(&origName)
		postureCheck.SetTypeID(rest_model.PostureCheckTypeMFA)

		origAttributes := rest_model.Attributes(s(postureCheckRole))
		postureCheck.SetRoleAttributes(&origAttributes)

		postureCheckBody, err := postureCheck.MarshalJSON()
		ctx.Req.NoError(err)

		resp := ctx.AdminManagementSession.createEntityOfType("posture-checks", postureCheckBody)

		standardJsonResponseTests(resp, http.StatusCreated, t)
		postureCheckId := ctx.getEntityId(resp.Body())
		ctx.Req.NotEmpty(postureCheckId)

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

			ctx.Req.Equal(postureCheckId, postureQueries[0].Path("id").Data().(string))
			ctx.Req.Equal(string(postureCheck.TypeID()), postureQueries[0].Path("queryType").Data().(string))

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
				ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
				ctx.Req.Equal(0, int(postureQueries[0].Path("timeoutRemaining").Data().(float64)))
			})
		})

		t.Run("cannot create session with failing queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
		})

		t.Run("providing posture data", func(t *testing.T) {
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

					ctx.Req.Equal(postureCheckId, postureQueries[0].Path("id").Data().(string))
					ctx.Req.Equal(string(postureCheck.TypeID()), postureQueries[0].Path("queryType").Data().(string))

					t.Run("query is currently passing", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
						ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
						ctx.Req.Greater(int(postureQueries[0].Path("timeout").Data().(float64)), 0)
					})
				})

				t.Run("sending woke state should lower timeout", func(t *testing.T) {
					wokeState := rest_model.PostureResponseEndpointStateCreate{
						Unlocked: false,
						Woken:    true,
					}
					id := uuid.New().String()
					wokeState.SetID(&id)

					resp, err := enrolledIdentitySession.NewRequest().SetBody(wokeState).Post("posture-response")

					ctx.Req.NoError(err)
					standardJsonResponseTests(resp, http.StatusCreated, t)

					t.Run("woke posture response should include service timeout change event", func(t *testing.T) {
						ctx.testContextChanged(t)

						responseEnvelope := &rest_model.PostureResponseEnvelope{}

						err := responseEnvelope.UnmarshalBinary(resp.Body())
						ctx.Req.NoError(err)

						ctx.Req.Len(responseEnvelope.Data.Services, 1)
						ctx.Req.Equal(origTimeout, *responseEnvelope.Data.Services[0].Timeout)
						ctx.Req.Equal(service.Name, *responseEnvelope.Data.Services[0].Name)
						ctx.Req.Equal(service.Id, *responseEnvelope.Data.Services[0].ID)
						ctx.Req.Equal("MFA", *responseEnvelope.Data.Services[0].PostureQueryType)
						ctx.Req.Greater(*responseEnvelope.Data.Services[0].TimeoutRemaining, int64(1))
						ctx.Req.LessOrEqual(*responseEnvelope.Data.Services[0].TimeoutRemaining, int64(300))
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

						ctx.Req.Equal(postureCheckId, postureQueries[0].Path("id").Data().(string))
						ctx.Req.Equal(string(postureCheck.TypeID()), postureQueries[0].Path("queryType").Data().(string))

						t.Run("query has a lower timeout", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
							ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
							ctx.Req.Greater(int(postureQueries[0].Path("timeoutRemaining").Data().(float64)), 0)
							ctx.Req.LessOrEqual(int(postureQueries[0].Path("timeoutRemaining").Data().(float64)), 300)
						})
					})
				})
			})

			t.Run("a new api session can pass MFA auth to create service session if there are no endpoint state changes", func(t *testing.T) {
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
			})

			t.Run("a new api session that is marked as unlocked can create a new session inside grace period", func(t *testing.T) {
				ctx.testContextChanged(t)

				newSession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)
				ctx.Req.NoError(err)

				resp, err := newSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(computeMFACode(mfaSecret))).Post("/authenticate/mfa")
				ctx.Req.NoError(err)

				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				postureResponseUnlocked := rest_model.PostureResponseEndpointStateCreate{
					Unlocked: true,
				}
				postureResponseUnlocked.SetID(&postureCheckId)

				newSession.requireCreateRestModelPostureResponse(postureResponseUnlocked)

				resp, err = newSession.createNewSession(service.Id)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
			})

			t.Run("a new api session that is marked as woken can create a new session inside the grace period", func(t *testing.T) {
				ctx.testContextChanged(t)

				newSession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)
				ctx.Req.NoError(err)

				resp, err := newSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(computeMFACode(mfaSecret))).Post("/authenticate/mfa")
				ctx.Req.NoError(err)

				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				postureResponseUnlocked := rest_model.PostureResponseEndpointStateCreate{
					Woken: true,
				}
				postureResponseUnlocked.SetID(&postureCheckId)

				newSession.requireCreateRestModelPostureResponse(postureResponseUnlocked)

				resp, err = newSession.createNewSession(service.Id)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
			})
		})
	})

	t.Run("can create an MFA posture check with an extremely low timeout", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))

		ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))

		ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

		const (
			lowMfaTimeout      = 5
			mfaTimeoutInterval = 5
		)

		origTimeout := int64(lowMfaTimeout)

		postureCheck := rest_model.PostureCheckMfaCreate{
			PostureCheckMfaProperties: rest_model.PostureCheckMfaProperties{
				PromptOnUnlock: true,
				PromptOnWake:   true,
				TimeoutSeconds: origTimeout,
			},
		}

		origName := uuid.New().String()
		postureCheck.SetName(&origName)
		postureCheck.SetTypeID(rest_model.PostureCheckTypeMFA)

		origAttributes := rest_model.Attributes(s(postureCheckRole))
		postureCheck.SetRoleAttributes(&origAttributes)

		postureCheckBody, err := postureCheck.MarshalJSON()
		ctx.Req.NoError(err)

		resp := ctx.AdminManagementSession.createEntityOfType("posture-checks", postureCheckBody)

		standardJsonResponseTests(resp, http.StatusCreated, t)
		postureCheckId := ctx.getEntityId(resp.Body())
		ctx.Req.NotEmpty(postureCheckId)

		t.Run("cannot create session with failing queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
		})

		t.Run("providing posture data", func(t *testing.T) {
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

					ctx.Req.Equal(postureCheckId, postureQueries[0].Path("id").Data().(string))
					ctx.Req.Equal(string(postureCheck.TypeID()), postureQueries[0].Path("queryType").Data().(string))

					t.Run("query is currently passing", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
						ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
						ctx.Req.Greater(int(postureQueries[0].Path("timeout").Data().(float64)), 0)
					})
				})
			})

			t.Run("a new api session with a low timeout MFA check can create a session", func(t *testing.T) {
				ctx.testContextChanged(t)

				newSession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)
				ctx.Req.NoError(err)

				resp, err := newSession.newAuthenticatedRequest().SetBody(newMfaCodeBody(computeMFACode(mfaSecret))).Post("/authenticate/mfa")
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				timeoutAt := time.Now().Add((mfaTimeoutInterval + lowMfaTimeout + 1) * time.Second)

				resp, err = newSession.createNewSession(service.Id)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

				sessionContainer, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.Req.True(sessionContainer.ExistsP("data.id"))
				sessionId, ok := sessionContainer.Path("data.id").Data().(string)
				ctx.Req.True(ok)
				ctx.Req.NotEmpty(sessionId)

				resp, err = ctx.AdminManagementSession.NewRequest().Get("/sessions/" + sessionId)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				resp, err = newSession.NewRequest().Get("/current-api-session/service-updates")
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				serviceUpdateContainer, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)

				ctx.Req.True(serviceUpdateContainer.ExistsP("data.lastChangeAt"))

				lastChangeAt, ok := serviceUpdateContainer.Path("data.lastChangeAt").Data().(string)
				ctx.Req.True(ok)
				ctx.Req.NotEmpty(lastChangeAt)

				t.Run("after the MFA posture check timeout", func(t *testing.T) {
					ctx.testContextChanged(t)

					durationTillTimeout := timeoutAt.Sub(time.Now())
					if durationTillTimeout > 0 {
						time.Sleep(durationTillTimeout)
					}

					t.Run("the last updated at serviced endpoint has changed", func(t *testing.T) {
						ctx.testContextChanged(t)

						resp, err = newSession.NewRequest().Get("/current-api-session/service-updates")
						ctx.Req.NoError(err)
						ctx.Req.Equal(http.StatusOK, resp.StatusCode())

						serviceUpdateContainer, err := gabs.ParseJSON(resp.Body())
						ctx.Req.NoError(err)

						ctx.Req.True(serviceUpdateContainer.ExistsP("data.lastChangeAt"))

						newLastChangeAt, ok := serviceUpdateContainer.Path("data.lastChangeAt").Data().(string)
						ctx.Req.True(ok)
						ctx.Req.NotEmpty(newLastChangeAt)

						ctx.Req.NotEqual(newLastChangeAt, lastChangeAt)
					})

					t.Run("the existing session was removed", func(t *testing.T) {
						ctx.testContextChanged(t)

						resp, err := ctx.AdminManagementSession.NewRequest().Get("/sessions/" + sessionId)
						ctx.Req.NoError(err)
						ctx.Req.Equal(http.StatusNotFound, resp.StatusCode())
					})

					t.Run("a new session cannot be created", func(t *testing.T) {
						resp, err = newSession.createNewSession(service.Id)
						ctx.Req.NoError(err)
						ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
					})
				})
			})
		})
	})

	t.Run("can create and patch an MFA posture check", func(t *testing.T) {
		ctx.testContextChanged(t)
		origTimeout := int64(60)
		postureCheck := rest_model.PostureCheckMfaCreate{
			PostureCheckMfaProperties: rest_model.PostureCheckMfaProperties{
				PromptOnUnlock: true,
				PromptOnWake:   true,
				TimeoutSeconds: origTimeout,
			},
		}

		origName := uuid.New().String()
		postureCheck.SetName(&origName)
		postureCheck.SetTypeID(rest_model.PostureCheckTypeMFA)

		origRole := uuid.New().String()
		origAttributes := rest_model.Attributes{
			origRole,
		}
		postureCheck.SetRoleAttributes(&origAttributes)

		origTags := map[string]interface{}{
			"keyOne": "valueOne",
		}
		postureCheck.SetTags(&rest_model.Tags{SubTags: origTags})

		postureCheckBody, err := postureCheck.MarshalJSON()
		ctx.Req.NoError(err)

		resp := ctx.AdminManagementSession.createEntityOfType("posture-checks", postureCheckBody)

		standardJsonResponseTests(resp, http.StatusCreated, t)
		id := ctx.getEntityId(resp.Body())
		ctx.Req.NotEmpty(id)

		t.Run("can be retrieved", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminManagementSession.NewRequest().Get("posture-checks/" + id)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			standardJsonResponseTests(resp, http.StatusOK, t)

			envelope := rest_model.DetailPostureCheckEnvelope{}

			err = envelope.UnmarshalJSON(resp.Body())
			ctx.Req.NoError(err)

			getAfterCreateCheck, ok := envelope.Data().(*rest_model.PostureCheckMfaDetail)
			ctx.Req.True(ok)
			ctx.Req.NotNil(getAfterCreateCheck)

			t.Run("has the correct values", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Equal(origName, *getAfterCreateCheck.Name())
				ctx.Req.ElementsMatch(s(origRole), *getAfterCreateCheck.RoleAttributes())
				ctx.Req.Len(getAfterCreateCheck.Tags().SubTags, 1)
				ctx.Req.Contains(getAfterCreateCheck.Tags().SubTags, "keyOne")
				ctx.Req.Equal("valueOne", getAfterCreateCheck.Tags().SubTags["keyOne"])
				ctx.Req.True(getAfterCreateCheck.PromptOnUnlock)
				ctx.Req.True(getAfterCreateCheck.PromptOnWake)
				ctx.Req.Equal(origTimeout, getAfterCreateCheck.TimeoutSeconds)
			})

			t.Run("can be patch timeout and promptOnUnlock", func(t *testing.T) {
				ctx.testContextChanged(t)

				patchTimeout := int64(30)
				f := false

				patchCheck := rest_model.PostureCheckMfaPatch{
					PostureCheckMfaPropertiesPatch: rest_model.PostureCheckMfaPropertiesPatch{
						PromptOnUnlock: &f,
						TimeoutSeconds: &patchTimeout,
					},
				}

				patchBody, err := patchCheck.MarshalJSON()
				ctx.Req.NoError(err)

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
					SetBody(patchBody).
					Patch("posture-checks/" + id)

				ctx.Req.NoError(err)

				standardJsonResponseTests(resp, http.StatusOK, t)

				t.Run("can be retrieved", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := ctx.AdminManagementSession.NewRequest().Get("posture-checks/" + id)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(resp)
					standardJsonResponseTests(resp, http.StatusOK, t)

					envelope := rest_model.DetailPostureCheckEnvelope{}

					err = envelope.UnmarshalJSON(resp.Body())
					ctx.Req.NoError(err)

					getAfterPatchOneCheck, ok := envelope.Data().(*rest_model.PostureCheckMfaDetail)
					ctx.Req.True(ok)
					ctx.Req.NotNil(getAfterPatchOneCheck)

					t.Run("has the correct values", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.Equal(origName, *getAfterPatchOneCheck.Name())
						ctx.Req.ElementsMatch(s(origRole), *getAfterPatchOneCheck.RoleAttributes())
						ctx.Req.Len(getAfterPatchOneCheck.Tags().SubTags, 1)
						ctx.Req.Contains(getAfterPatchOneCheck.Tags().SubTags, "keyOne")
						ctx.Req.Equal("valueOne", getAfterPatchOneCheck.Tags().SubTags["keyOne"])
						ctx.Req.False(getAfterPatchOneCheck.PromptOnUnlock)
						ctx.Req.True(getAfterPatchOneCheck.PromptOnWake)
						ctx.Req.Equal(patchTimeout, getAfterPatchOneCheck.TimeoutSeconds)
					})
				})
			})
		})
	})
}
