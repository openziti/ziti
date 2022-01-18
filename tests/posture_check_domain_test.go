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
	"github.com/Jeffail/gabs"
	"github.com/google/uuid"
	"github.com/openziti/edge/eid"
	"net/http"
	"testing"
)

func Test_PostureChecks_Domain(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	t.Run("can CRUD domain posture checks", func(t *testing.T) {
		ctx.testContextChanged(t)
		domain := "domain1"
		postureCheckRole := uuid.New().String()

		t.Run("can create a posture check", func(t *testing.T) {
			ctx.testContextChanged(t)
			postureCheck := ctx.AdminManagementSession.requireNewPostureCheckDomain(s(domain), s(postureCheckRole))

			t.Run("created posture check can have name patched", func(t *testing.T) {
				ctx.testContextChanged(t)
				putContainer := gabs.New()
				newName := "newName-" + uuid.New().String()
				_, _ = putContainer.Set(newName, "name")
				_, _ = putContainer.Set(postureCheck.typeId, "typeId")

				resp := ctx.AdminManagementSession.updateEntityOfType(postureCheck.id, postureCheck.getEntityType(), putContainer.String(), true)
				ctx.Req.NotNil(resp)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				updatedContainer := ctx.AdminManagementSession.requireQuery("/posture-checks/" + postureCheck.id)

				ctx.Req.Equal(newName, updatedContainer.Path("data.name").Data().(string), "name is patched")
				domains, err := updatedContainer.Path("data.domains").Children()
				ctx.Req.NoError(err)
				ctx.Req.Len(domains, 1)
				ctx.Req.Equal(domain, domains[0].Data().(string), "domain is previous value")

			})

			t.Run("created posture check can have tags patched", func(t *testing.T) {
				ctx.testContextChanged(t)
				putContainer := gabs.New()

				tags := map[string]string{
					"tag1": "value1",
				}
				_, _ = putContainer.Set(tags, "tags")
				_, _ = putContainer.Set(postureCheck.typeId, "typeId")

				resp := ctx.AdminManagementSession.updateEntityOfType(postureCheck.id, postureCheck.getEntityType(), putContainer.String(), true)
				ctx.Req.NotNil(resp)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				updatedContainer := ctx.AdminManagementSession.requireQuery("/posture-checks/" + postureCheck.id)

				updatedDomains, err := updatedContainer.Path("data.domains").Children()
				ctx.Req.NoError(err)
				ctx.Req.Len(updatedDomains, 1)
				ctx.Req.Equal(domain, updatedDomains[0].Data().(string), "domain is previous value")

				updatedTags, ok := updatedContainer.Path("data.tags").Data().(map[string]interface{})
				ctx.Req.True(ok, "has a data.tags attribute")
				ctx.Req.Equal("value1", updatedTags["tag1"], "has the updated tag values")
			})

			t.Run("created posture check can be deleted", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.AdminManagementSession.requireDeleteEntity(postureCheck)
			})
		})
	})

	t.Run("can create a domain posture check associated to a service", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		domain := "domain1"
		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckDomain(s(domain), s(postureCheckRole))

		dialPolicy := ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))
		bindPolicy := ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Bind", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))

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
			ctx.Req.Len(querySet, 2)

			var dialSet *gabs.Container
			var bindSet *gabs.Container
			if querySet[0].Path("policyId").Data().(string) == dialPolicy.id {
				dialSet = querySet[0]
				bindSet = querySet[1]
			} else {
				dialSet = querySet[1]
				bindSet = querySet[0]
			}

			ctx.Req.Equal(dialPolicy.policyType, dialSet.Path("policyType").Data().(string))
			ctx.Req.Equal(bindPolicy.policyType, bindSet.Path("policyType").Data().(string))

			postureQueries, err := dialSet.Path("postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(postureQueries, 1)

			ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
			ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(dialSet.Path("isPassing").Data().(bool))
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

			beforeLastChangedAt := enrolledIdentitySession.getServiceUpdateTime()

			enrolledIdentitySession.requireNewPostureResponseDomain(postureCheck.id, domain)

			afterLastChangedAt := enrolledIdentitySession.getServiceUpdateTime()

			t.Run("service update changed with posture data change", func(t *testing.T) {
				ctx.Req.True(afterLastChangedAt.After(beforeLastChangedAt), "expected last changed at to have been updated after posture submissions")
			})

			t.Run("allows posture checks to pass", func(t *testing.T) {
				ctx.testContextChanged(t)
				code, body := enrolledIdentitySession.query("/services/" + service.Id)
				ctx.Req.Equal(http.StatusOK, code)
				entityService, err := gabs.ParseJSON(body)
				ctx.Req.NoError(err)

				querySet, err := entityService.Path("data.postureQueries").Children()
				ctx.Req.NoError(err)
				ctx.Req.Len(querySet, 2)

				var dialSet *gabs.Container
				var bindSet *gabs.Container
				if querySet[0].Path("policyId").Data().(string) == dialPolicy.id {
					dialSet = querySet[0]
					bindSet = querySet[1]
				} else {
					dialSet = querySet[1]
					bindSet = querySet[0]
				}

				t.Run("dial policy passes", func(t *testing.T) {
					ctx.testContextChanged(t)
					postureQueries, err := dialSet.Path("postureQueries").Children()
					ctx.Req.NoError(err)
					ctx.Req.Len(postureQueries, 1)

					ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
					ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

					ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
					ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
				})

				t.Run("bind policy passes", func(t *testing.T) {
					ctx.testContextChanged(t)
					postureQueries, err := bindSet.Path("postureQueries").Children()
					ctx.Req.NoError(err)
					ctx.Req.Len(postureQueries, 1)

					ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
					ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

					ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
					ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
				})

			})
		})

		t.Run("can create session after posture data is provided", func(t *testing.T) {
			ctx.testContextChanged(t)
			existingSessionId := enrolledIdentitySession.requireNewSession(service.Id)
			ctx.Req.NotEmpty(existingSessionId)

			t.Run("post invalid posture data", func(t *testing.T) {
				ctx.testContextChanged(t)
				enrolledIdentitySession.requireNewPostureResponseDomain(postureCheck.id, "this domain is wrong")

				t.Run("query is failing passing", func(t *testing.T) {
					ctx.testContextChanged(t)
					code, body := enrolledIdentitySession.query("/services/" + service.Id)
					ctx.Req.Equal(http.StatusOK, code)
					entityService, err := gabs.ParseJSON(body)
					ctx.Req.NoError(err)

					querySet, err := entityService.Path("data.postureQueries").Children()
					ctx.Req.NoError(err)
					ctx.Req.Len(querySet, 2)

					var dialSet *gabs.Container
					var bindSet *gabs.Container
					if querySet[0].Path("policyId").Data().(string) == dialPolicy.id {
						dialSet = querySet[0]
						bindSet = querySet[1]
					} else {
						dialSet = querySet[1]
						bindSet = querySet[0]
					}

					t.Run("dial passes", func(t *testing.T) {
						ctx.testContextChanged(t)
						postureQueries, err := dialSet.Path("postureQueries").Children()
						ctx.Req.NoError(err)
						ctx.Req.Len(postureQueries, 1)

						ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
						ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

						ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
						ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
					})

					t.Run("bind passes", func(t *testing.T) {
						ctx.testContextChanged(t)
						postureQueries, err := bindSet.Path("postureQueries").Children()
						ctx.Req.NoError(err)
						ctx.Req.Len(postureQueries, 1)

						ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
						ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

						ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
						ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
					})

				})
			})

			t.Run("previously existing session no longer exists", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotEmpty(existingSessionId)
				enrolledIdentitySession.requireNotFoundEntityLookup("sessions", existingSessionId)
			})
		})

		t.Run("providing valid posture data via bulk endpoint", func(t *testing.T) {
			ctx.testContextChanged(t)
			newSession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)
			ctx.Req.NoError(err)

			newSession.requireNewPostureResponseBulkDomain(postureCheck.id, domain)

			t.Run("allows posture checks to pass", func(t *testing.T) {
				ctx.testContextChanged(t)
				code, body := enrolledIdentitySession.query("/services/" + service.Id)
				ctx.Req.Equal(http.StatusOK, code)
				entityService, err := gabs.ParseJSON(body)
				ctx.Req.NoError(err)

				querySet, err := entityService.Path("data.postureQueries").Children()
				ctx.Req.NoError(err)
				ctx.Req.Len(querySet, 2)

				var dialSet *gabs.Container
				var bindSet *gabs.Container
				if querySet[0].Path("policyId").Data().(string) == dialPolicy.id {
					dialSet = querySet[0]
					bindSet = querySet[1]
				} else {
					dialSet = querySet[1]
					bindSet = querySet[0]
				}

				t.Run("dial passes", func(t *testing.T) {
					ctx.testContextChanged(t)

					postureQueries, err := dialSet.Path("postureQueries").Children()
					ctx.Req.NoError(err)
					ctx.Req.Len(postureQueries, 1)

					ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
					ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

					ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
					ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
				})

				t.Run("bind passes", func(t *testing.T) {
					ctx.testContextChanged(t)

					postureQueries, err := bindSet.Path("postureQueries").Children()
					ctx.Req.NoError(err)
					ctx.Req.Len(postureQueries, 1)

					ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
					ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

					ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
					ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
				})
			})
		})
	})

	t.Run("can create a domain posture check associated to a service policy and have service policy with no posture checks", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		domain := "domain1"
		_ = ctx.AdminManagementSession.requireNewPostureCheckDomain(s(domain), s(postureCheckRole))

		ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))
		ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), nil)

		ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))

		ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

		ctx.Req.NoError(err)

		t.Run("identity can see service via policies", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(enrolledIdentitySession.isServiceVisibleToUser(service.Id))
		})

		t.Run("can create session with failing queries via one service policy by having another service policy with no checks", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
		})
	})
}
