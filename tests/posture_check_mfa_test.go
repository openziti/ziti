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
	"fmt"
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
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

		postureCheckId := ctx.getEntityId(resp.Body())
		ctx.Req.NotEmpty(postureCheckId)

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().Get("/posture-checks/" + postureCheckId)
		ctx.Req.NoError(err)

		responseEnvelope := rest_model.DetailPostureCheckEnvelope{}

		err = responseEnvelope.UnmarshalJSON(resp.Body())
		ctx.Req.NoError(err)

		postureCheckDetail := responseEnvelope.Data().(*rest_model.PostureCheckMfaDetail)

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

			serviceEnvelope := &rest_model.DetailServiceEnvelope{}
			err := serviceEnvelope.UnmarshalBinary(body)
			ctx.Req.NoError(err)

			querySet := serviceEnvelope.Data.PostureQueries
			ctx.Req.NotNil(querySet)
			ctx.Req.NotEmpty(querySet)
			ctx.Req.Len(querySet, 1)

			postureQueries := querySet[0].PostureQueries
			ctx.Req.NoError(err)
			ctx.Req.Len(postureQueries, 1)

			ctx.Req.Equal(postureCheckId, *postureQueries[0].ID)
			ctx.Req.Equal(postureCheck.TypeID(), *postureQueries[0].QueryType)

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(*querySet[0].IsPassing)
				ctx.Req.False(*postureQueries[0].IsPassing)
				ctx.Req.Equal(int64(-1), *postureQueries[0].Timeout)
			})

			t.Run("updatedAt equals posture check createdAt", func(t *testing.T) {
				//fix this
				ctx.testContextChanged(t)
				ctx.Req.NotNil(postureCheckDetail)
				ctx.Req.NotEmpty(postureCheckDetail.CreatedAt())
				postureCheckDetail.CreatedAt().Equal(*postureQueries[0].CreatedAt)
			})
		})

		t.Run("cannot create session with failing queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusConflict, resp.StatusCode())

			t.Run("failed service request exist", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/identities/" + enrolledIdentitySession.identityId + "/failed-service-requests")
				ctx.Req.NoError(err)

				listEnvelop := rest_model.FailedServiceRequestEnvelope{}
				ctx.Req.NoError(listEnvelop.UnmarshalBinary(resp.Body()))

				ctx.Req.NotEmpty(listEnvelop.Data)
			})
		})

		t.Run("providing valid posture data", func(t *testing.T) {
			ctx.testContextChanged(t)

			mfaSecret := ""

			t.Run("by starting enrollment in MFA", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := enrolledIdentitySession.newAuthenticatedRequest().Post("/current-identity/mfa")

				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

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
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				mfaEnvelope := &rest_model.DetailMfaEnvelope{}

				err = mfaEnvelope.UnmarshalBinary(resp.Body())
				ctx.Req.NoError(err)
				ctx.Req.NotNil(mfaEnvelope)
				ctx.Req.NotNil(mfaEnvelope.Data)

				mfa := mfaEnvelope.Data
				ctx.Req.NotNil(mfa.ID)
				ctx.Req.NotEmpty(mfa.ProvisioningURL)

				parsedUrl, err := url.Parse(mfa.ProvisioningURL)
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
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

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
					serviceEnvelope := &rest_model.DetailServiceEnvelope{}
					err = serviceEnvelope.UnmarshalBinary(body)
					ctx.Req.NoError(err)

					entityService := serviceEnvelope.Data

					querySet := entityService.PostureQueries
					ctx.Req.Len(querySet, 1)

					postureQueries := querySet[0].PostureQueries
					ctx.Req.Len(postureQueries, 1)

					ctx.Req.Equal(postureCheckId, *postureQueries[0].ID)
					ctx.Req.Equal(postureCheck.TypeID(), *postureQueries[0].QueryType)

					t.Run("query is currently passing", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.True(*querySet[0].IsPassing)
						ctx.Req.True(*postureQueries[0].IsPassing)
						ctx.Req.Equal(int64(-1), *postureQueries[0].Timeout)
					})

					t.Run("updatedAt equals posture check createdAt", func(t *testing.T) {
						//fix this
						ctx.testContextChanged(t)
						ctx.Req.NotNil(postureCheckDetail)
						ctx.Req.NotEmpty(postureCheckDetail.CreatedAt())
						postureCheckDetail.CreatedAt().Equal(*postureQueries[0].CreatedAt)
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

				sessionEnvelope := &rest_model.SessionCreateEnvelope{}
				err = sessionEnvelope.UnmarshalBinary(resp.Body())
				ctx.Req.NoError(err)

				ctx.Req.NotNil(sessionEnvelope.Data.ID)

				sessionId := *sessionEnvelope.Data.ID
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

		ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
		postureCheckId := ctx.getEntityId(resp.Body())
		ctx.Req.NotEmpty(postureCheckId)

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().Get("/posture-checks/" + postureCheckId)
		ctx.Req.NoError(err)

		responseEnvelope := rest_model.DetailPostureCheckEnvelope{}

		err = responseEnvelope.UnmarshalJSON(resp.Body())
		ctx.Req.NoError(err)

		postureCheckDetail := responseEnvelope.Data().(*rest_model.PostureCheckMfaDetail)

		t.Run("identity can see service via policies", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(enrolledIdentitySession.isServiceVisibleToUser(service.Id))
		})

		t.Run("service has the posture check in its queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			code, body := enrolledIdentitySession.query("/services/" + service.Id)
			ctx.Req.Equal(http.StatusOK, code)

			serviceEnvelope := &rest_model.DetailServiceEnvelope{}
			err = serviceEnvelope.UnmarshalBinary(body)
			ctx.Req.NoError(err)

			entityService := serviceEnvelope.Data
			ctx.Req.NotNil(entityService)
			ctx.Req.NotNil(entityService.ID)

			querySet := entityService.PostureQueries
			ctx.Req.Len(querySet, 1)

			postureQueries := querySet[0].PostureQueries
			ctx.Req.Len(postureQueries, 1)

			ctx.Req.Equal(postureCheckId, *postureQueries[0].ID)
			ctx.Req.Equal(postureCheck.TypeID(), *postureQueries[0].QueryType)

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(*querySet[0].IsPassing)
				ctx.Req.False(*postureQueries[0].IsPassing)
				ctx.Req.Equal(int64(0), *postureQueries[0].TimeoutRemaining)
			})

			t.Run("updatedAt equals posture check createdAt", func(t *testing.T) {
				//fix this
				ctx.testContextChanged(t)
				ctx.Req.NotNil(postureCheckDetail)
				ctx.Req.NotEmpty(postureCheckDetail.CreatedAt())
				postureCheckDetail.CreatedAt().Equal(*postureQueries[0].CreatedAt)
			})
		})

		t.Run("cannot create session with failing queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusConflict, resp.StatusCode())

			t.Run("failed service request exist", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/identities/" + enrolledIdentitySession.identityId + "/failed-service-requests")
				ctx.Req.NoError(err)

				listEnvelop := &rest_model.FailedServiceRequestEnvelope{}
				ctx.Req.NoError(listEnvelop.UnmarshalBinary(resp.Body()))
				ctx.Req.NotEmpty(listEnvelop.Data)
			})
		})

		t.Run("providing posture data", func(t *testing.T) {
			ctx.testContextChanged(t)

			mfaSecret := ""

			t.Run("by starting enrollment in MFA", func(t *testing.T) {
				ctx.testContextChanged(t)

				resp, err := enrolledIdentitySession.newAuthenticatedRequest().Post("/current-identity/mfa")

				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

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
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				mfaEnvelope := &rest_model.DetailMfaEnvelope{}
				err = mfaEnvelope.UnmarshalBinary(resp.Body())
				ctx.Req.NoError(err)
				ctx.Req.NotNil(mfaEnvelope)
				ctx.Req.NotNil(mfaEnvelope.Data)

				mfa := mfaEnvelope.Data
				ctx.Req.NotNil(mfa.ID)
				ctx.Req.NotEmpty(mfa.ProvisioningURL)

				parsedUrl, err := url.Parse(mfa.ProvisioningURL)
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
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

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

					serviceEnvelope := &rest_model.DetailServiceEnvelope{}
					err := serviceEnvelope.UnmarshalBinary(body)
					ctx.Req.NoError(err)

					querySet := serviceEnvelope.Data.PostureQueries
					ctx.Req.NotNil(querySet)
					ctx.Req.NotEmpty(querySet)
					ctx.Req.Len(querySet, 1)

					postureQueries := querySet[0].PostureQueries
					ctx.Req.NoError(err)
					ctx.Req.Len(postureQueries, 1)

					ctx.Req.Equal(postureCheckId, *postureQueries[0].ID)
					ctx.Req.Equal(postureCheck.TypeID(), *postureQueries[0].QueryType)

					t.Run("query is currently passing", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.True(*querySet[0].IsPassing)
						ctx.Req.True(*postureQueries[0].IsPassing)
						ctx.Req.Greater(*postureQueries[0].Timeout, int64(0))
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
					ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

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

						serviceEnvelope := &rest_model.DetailServiceEnvelope{}
						err := serviceEnvelope.UnmarshalBinary(body)
						ctx.Req.NoError(err)

						querySet := serviceEnvelope.Data.PostureQueries
						ctx.Req.NotNil(querySet)
						ctx.Req.NotEmpty(querySet)
						ctx.Req.Len(querySet, 1)

						postureQueries := querySet[0].PostureQueries
						ctx.Req.NoError(err)
						ctx.Req.Len(postureQueries, 1)

						ctx.Req.Equal(postureCheckId, *postureQueries[0].ID)
						ctx.Req.Equal(postureCheck.TypeID(), *postureQueries[0].QueryType)

						t.Run("query has a lower timeout", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.True(*querySet[0].IsPassing)
							ctx.Req.True(*postureQueries[0].IsPassing)
							ctx.Req.Greater(*postureQueries[0].TimeoutRemaining, int64(0))
							ctx.Req.LessOrEqual(*postureQueries[0].TimeoutRemaining, int64(300))
						})

						t.Run("query has a new updatedAt value", func(t *testing.T) {
							ctx.testContextChanged(t)
							ctx.Req.True(time.Time(*postureQueries[0].UpdatedAt).After(time.Time(*postureCheckDetail.UpdatedAt())), "posture query updated at [%s] should now be after original time [%s]", postureQueries[0].UpdatedAt.String(), postureCheckDetail.UpdatedAt().String())
						})

						t.Run("updating the posture check further advances updatedAt", func(t *testing.T) {
							ctx.testContextChanged(t)

							newTimeout := int64(600)
							mfaPatch := &rest_model.PostureCheckMfaPatch{}
							mfaPatch.TimeoutSeconds = &newTimeout

							body, err := mfaPatch.MarshalJSON()
							ctx.Req.NoError(err)

							resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(body).Patch("/posture-checks/" + postureCheckId)
							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusOK, resp.StatusCode())

							resp, err = enrolledIdentitySession.newAuthenticatedRequest().Get("/services/" + service.Id)
							ctx.Req.NoError(err)
							ctx.Req.Equal(http.StatusOK, resp.StatusCode())

							patchedServiceEnv := &rest_model.DetailServiceEnvelope{}
							err = patchedServiceEnv.UnmarshalBinary(resp.Body())
							ctx.Req.NoError(err)

							patchedQuerySet := patchedServiceEnv.Data.PostureQueries
							ctx.Req.NotNil(patchedQuerySet)
							ctx.Req.NotEmpty(patchedQuerySet)
							ctx.Req.Len(patchedQuerySet, 1)

							patchedPostureQueries := patchedQuerySet[0].PostureQueries
							ctx.Req.NoError(err)
							ctx.Req.Len(patchedPostureQueries, 1)

							ctx.Req.Equal(postureCheckId, *patchedPostureQueries[0].ID)
							ctx.Req.Equal(postureCheck.TypeID(), *patchedPostureQueries[0].QueryType)

							ctx.Req.True(time.Time(*patchedPostureQueries[0].UpdatedAt).After(time.Time(*postureQueries[0].UpdatedAt)), "posture query updated at [%s] should now be after state change time [%s]", patchedPostureQueries[0].UpdatedAt.String(), postureCheckDetail.UpdatedAt().String())
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

				serviceEnvelope := &rest_model.DetailServiceEnvelope{}
				err = serviceEnvelope.UnmarshalBinary(resp.Body())
				ctx.Req.NoError(err)

				entityService := serviceEnvelope.Data
				ctx.Req.NotNil(entityService)
				ctx.Req.NotNil(entityService.ID)

				sessionId := *entityService.ID
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

		ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
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
				ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

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
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				mfaEnvelope := &rest_model.DetailMfaEnvelope{}

				err = mfaEnvelope.UnmarshalBinary(resp.Body())
				ctx.Req.NoError(err)
				ctx.Req.NotNil(mfaEnvelope)
				ctx.Req.NotNil(mfaEnvelope.Data)

				mfa := mfaEnvelope.Data
				ctx.Req.NotNil(mfa.ID)
				ctx.Req.NotEmpty(mfa.ProvisioningURL)

				parsedUrl, err := url.Parse(mfa.ProvisioningURL)
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
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

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

					serviceEnvelope := &rest_model.DetailServiceEnvelope{}
					err := serviceEnvelope.UnmarshalBinary(body)
					ctx.Req.NoError(err)

					querySet := serviceEnvelope.Data.PostureQueries
					ctx.Req.NotNil(querySet)
					ctx.Req.NotEmpty(querySet)
					ctx.Req.Len(querySet, 1)

					postureQueries := querySet[0].PostureQueries
					ctx.Req.NoError(err)
					ctx.Req.Len(postureQueries, 1)

					ctx.Req.Equal(postureCheckId, *postureQueries[0].ID)
					ctx.Req.Equal(postureCheck.TypeID(), *postureQueries[0].QueryType)

					t.Run("query is currently passing", func(t *testing.T) {
						ctx.testContextChanged(t)
						ctx.Req.True(*querySet[0].IsPassing)
						ctx.Req.True(*postureQueries[0].IsPassing)
						ctx.Req.Greater(*postureQueries[0].Timeout, int64(0))
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

				sessionCreateEnvelope := &rest_model.SessionCreateEnvelope{}
				err = sessionCreateEnvelope.UnmarshalBinary(resp.Body())
				ctx.Req.NoError(err)
				ctx.Req.NotNil(sessionCreateEnvelope.Data)
				ctx.Req.NotNil(sessionCreateEnvelope.Data.ID)

				sessionId := *sessionCreateEnvelope.Data.ID
				ctx.Req.NotEmpty(sessionId)

				resp, err = ctx.AdminManagementSession.NewRequest().Get("/sessions/" + sessionId)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())
				ctx.Req.NoError(err)

				sessionDetailEnvelope := &rest_model.DetailSessionEnvelope{}
				err = sessionDetailEnvelope.UnmarshalBinary(resp.Body())
				ctx.Req.NoError(err)
				ctx.Req.NotNil(sessionDetailEnvelope.Data)
				ctx.Req.NotNil(sessionDetailEnvelope.Data.ID)

				resp, err = newSession.NewRequest().Get("/current-api-session/service-updates")
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				serviceUpdatesEnvelop := &rest_model.ListCurrentAPISessionServiceUpdatesEnvelope{}
				err = serviceUpdatesEnvelop.UnmarshalBinary(resp.Body())
				ctx.Req.NoError(err)
				ctx.Req.NotNil(serviceUpdatesEnvelop.Data)
				ctx.Req.NotNil(serviceUpdatesEnvelop.Data.LastChangeAt)
				ctx.Req.False(time.Time(*serviceUpdatesEnvelop.Data.LastChangeAt).IsZero())

				originalLastChangedAt := *serviceUpdatesEnvelop.Data.LastChangeAt

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

						serviceUpdatesEnvelop := &rest_model.ListCurrentAPISessionServiceUpdatesEnvelope{}
						err = serviceUpdatesEnvelop.UnmarshalBinary(resp.Body())
						ctx.Req.NoError(err)
						ctx.Req.NotNil(serviceUpdatesEnvelop.Data)
						ctx.Req.NotNil(serviceUpdatesEnvelop.Data.LastChangeAt)
						ctx.Req.False(time.Time(*serviceUpdatesEnvelop.Data.LastChangeAt).IsZero())

						newLastChangeAt := *serviceUpdatesEnvelop.Data.LastChangeAt

						ctx.Req.False(originalLastChangedAt.Equal(newLastChangeAt))
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

		ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
		id := ctx.getEntityId(resp.Body())
		ctx.Req.NotEmpty(id)

		t.Run("can be retrieved", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminManagementSession.NewRequest().Get("posture-checks/" + id)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

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

				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				t.Run("can be retrieved", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := ctx.AdminManagementSession.NewRequest().Get("posture-checks/" + id)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(resp)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

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
