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
	"github.com/openziti/edge/rest_model"
	"net/http"
	"testing"
)

func Test_Router_ReEnroll(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("unenrolled router", func(t *testing.T) {
		ctx.testContextChanged(t)
		edgeRouter := ctx.requireCreateEdgeRouter(false)

		t.Run("can start re-enrollment", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody("{}").Post("edge-routers/" + edgeRouter.id + "/re-enroll")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			edgeRouterDetail := &rest_model.EdgeRouterDetail{}
			envelope := &rest_model.DetailedEdgeRouterEnvelope{
				Data: edgeRouterDetail,
				Meta: &rest_model.Meta{},
			}
			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(envelope).Get("edge-routers/" + edgeRouter.id)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
			ctx.Req.NotNil(edgeRouterDetail.EnrollmentJwt)

			t.Run("router has no current fingerprint", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Empty(edgeRouterDetail.Fingerprint)
			})

			t.Run("router has a new enrollment JWT", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(edgeRouterDetail.EnrollmentJwt)

				t.Run("router can enroll", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.requireEnrollEdgeRouter(false, *edgeRouterDetail.ID)
				})
			})
		})
	})

	t.Run("enrolled router", func(t *testing.T) {

		enrolledRouter := ctx.createAndEnrollEdgeRouter(false)

		t.Run("has no current enrollment", func(t *testing.T) {
			ctx.testContextChanged(t)

			edgeRouterDetail := &rest_model.EdgeRouterDetail{}
			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(edgeRouterDetail).Get("edge-routers/" + enrolledRouter.id)

			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
			ctx.Req.Nil(edgeRouterDetail.EnrollmentJwt)
		})

		t.Run("can start re-enroll", func(t *testing.T) {
			ctx.testContextChanged(t)

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody("{}").Post("edge-routers/" + enrolledRouter.id + "/re-enroll")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			edgeRouterDetail := &rest_model.EdgeRouterDetail{}
			envelope := &rest_model.DetailedEdgeRouterEnvelope{
				Data: edgeRouterDetail,
				Meta: &rest_model.Meta{},
			}
			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(envelope).Get("edge-routers/" + enrolledRouter.id)

			ctx.Req.NoError(err)
			ctx.Req.NotNil(resp)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())
			ctx.Req.NotNil(edgeRouterDetail.EnrollmentJwt)

			t.Run("router has no current fingerprint", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Empty(edgeRouterDetail.Fingerprint)
			})

			t.Run("router has a new enrollment JWT", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NotNil(edgeRouterDetail.EnrollmentJwt)

				t.Run("router can enroll with JWT", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.requireEnrollEdgeRouter(false, *edgeRouterDetail.ID)
				})
			})
		})
	})
}
