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
	"github.com/openziti/edge/eid"
	"net/http"
	"testing"
)

func Test_PostureChecks(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	t.Run("can create a domain posture check associated to a service", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.Authenticate(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminSession.requireNewService(s(serviceRole), nil)

		domain := "domain1"
		postureCheck := ctx.AdminSession.requireNewPostureCheckDomain(s(domain), s(postureCheckRole))

		ctx.AdminSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))

		ctx.AdminSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))

		ctx.AdminSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

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
			enrolledIdentitySession.requireNewPostureResponseDomain(postureCheck.id, domain)

			t.Run("allows posture checks to pass", func(t *testing.T) {
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

				ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
				ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
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
					ctx.Req.Len(querySet, 1)

					postureQueries, err := querySet[0].Path("postureQueries").Children()
					ctx.Req.NoError(err)
					ctx.Req.Len(postureQueries, 1)

					ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
					ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

					ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
					ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
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
			newSession, err := enrolledIdentityAuthenticator.Authenticate(ctx)
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
				ctx.Req.Len(querySet, 1)

				postureQueries, err := querySet[0].Path("postureQueries").Children()
				ctx.Req.NoError(err)
				ctx.Req.Len(postureQueries, 1)

				ctx.Req.Equal(postureCheck.id, postureQueries[0].Path("id").Data().(string))
				ctx.Req.Equal(postureCheck.typeId, postureQueries[0].Path("queryType").Data().(string))

				ctx.Req.True(querySet[0].Path("isPassing").Data().(bool))
				ctx.Req.True(postureQueries[0].Path("isPassing").Data().(bool))
			})
		})
	})
}
