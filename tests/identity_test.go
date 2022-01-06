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
	"github.com/openziti/foundation/util/stringz"
	"net/http"
	"net/url"
	"sort"
	"testing"
)

func Test_Identity(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("role attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		identity := newTestIdentity(false, role1, role2)
		identity.Id = ctx.AdminManagementSession.requireCreateEntity(identity)
		ctx.AdminManagementSession.validateEntityWithQuery(identity)
		ctx.AdminManagementSession.validateEntityWithLookup(identity)
	})

	t.Run("service hosting values should be set", func(t *testing.T) {
		ctx.testContextChanged(t)
		svc1 := ctx.AdminManagementSession.requireNewService(nil, nil)
		svc2 := ctx.AdminManagementSession.requireNewService(nil, nil)

		identity := newTestIdentity(false)
		identity.defaultHostingPrecedence = "required"
		identity.defaultHostingCost = 150
		identity.serviceHostingPrecedences = map[string]interface{}{
			svc1.Id: "required",
			svc2.Id: "failed",
		}

		identity.serviceHostingCosts = map[string]uint16{
			svc1.Id: 200,
			svc2.Id: 300,
		}

		identity.Id = ctx.AdminManagementSession.requireCreateEntity(identity)
		ctx.AdminManagementSession.validateEntityWithQuery(identity)
		ctx.AdminManagementSession.validateEntityWithLookup(identity)
	})

	t.Run("role attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		identity := newTestIdentity(false, role1, role2)
		identity.Id = ctx.AdminManagementSession.requireCreateEntity(identity)

		role3 := eid.New()
		identity.roleAttributes = []string{role2, role3}
		ctx.AdminManagementSession.requireUpdateEntity(identity)
		ctx.AdminManagementSession.validateEntityWithLookup(identity)
	})

	t.Run("role attributes should not be changed on PATCH if not sent", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		identity := newTestIdentity(false, role1, role2)
		identity.Id = ctx.AdminManagementSession.requireCreateEntity(identity)

		patchContainer := gabs.New()
		newName := uuid.New().String()
		_, _ = patchContainer.Set(newName, "name")
		identity.name = newName

		resp := ctx.AdminManagementSession.updateEntityOfType(identity.Id, identity.getEntityType(), patchContainer.String(), true)

		ctx.Req.NotNil(resp)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		updatedIdentity := ctx.AdminManagementSession.requireQuery("identities/" + identity.Id)

		ctx.Req.Equal(newName, updatedIdentity.Path("data.name").Data().(string), "name should be updated")

		updateAttributes, err := updatedIdentity.Path("data.roleAttributes").Children()
		ctx.Req.NoError(err)

		var list []string

		for _, attr := range updateAttributes {
			if attrString, ok := attr.Data().(string); ok {
				list = append(list, attrString)
			}
		}
		ctx.Req.True(stringz.ContainsAll(list, role1, role2), "retained original attributes")
	})

	t.Run("role attributes should be changed on PATCH if sent", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		identity := newTestIdentity(false, role1, role2)
		identity.Id = ctx.AdminManagementSession.requireCreateEntity(identity)

		patchContainer := gabs.New()

		role3 := eid.New()
		_, _ = patchContainer.Set([]string{role1, role2, role3}, "roleAttributes")

		resp := ctx.AdminManagementSession.updateEntityOfType(identity.Id, identity.getEntityType(), patchContainer.String(), true)

		ctx.Req.NotNil(resp)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		updatedIdentity := ctx.AdminManagementSession.requireQuery("identities/" + identity.Id)

		updateAttributes, err := updatedIdentity.Path("data.roleAttributes").Children()
		ctx.Req.NoError(err)

		var list []string

		for _, attr := range updateAttributes {
			if attrString, ok := attr.Data().(string); ok {
				list = append(list, attrString)
			}
		}
		ctx.Req.True(stringz.ContainsAll(list, role1, role2, role3), "role attributes updated")
	})

	t.Run("role attributes should be queryable", func(t *testing.T) {
		ctx.testContextChanged(t)
		prefix := "rol3-attribut3-qu3ry-t3st-"
		role1 := prefix + "sales"
		role2 := prefix + "support"
		role3 := prefix + "engineering"
		role4 := prefix + "field-ops"
		role5 := prefix + "executive"

		ctx.AdminManagementSession.requireNewIdentity(false, role1, role2)
		ctx.AdminManagementSession.requireNewIdentity(false, role2, role3)
		ctx.AdminManagementSession.requireNewIdentity(false, role3, role4)
		identity := ctx.AdminManagementSession.requireNewIdentity(false, role5)
		ctx.AdminManagementSession.requireNewIdentity(false)

		list := ctx.AdminManagementSession.requireList("identity-role-attributes")
		ctx.Req.True(len(list) >= 5)
		ctx.Req.True(stringz.ContainsAll(list, role1, role2, role3, role4, role5))

		filter := url.QueryEscape(`id contains "e" and id contains "` + prefix + `" sort by id`)
		list = ctx.AdminManagementSession.requireList("identity-role-attributes?filter=" + filter)
		ctx.Req.Equal(4, len(list))

		expected := []string{role1, role3, role4, role5}
		sort.Strings(expected)
		ctx.Req.Equal(expected, list)

		identity.roleAttributes = nil
		ctx.AdminManagementSession.requireUpdateEntity(identity)
		list = ctx.AdminManagementSession.requireList("identity-role-attributes")
		ctx.Req.True(len(list) >= 4)
		ctx.Req.True(stringz.ContainsAll(list, role1, role2, role3, role4))
		ctx.Req.False(stringz.Contains(list, role5))
	})

	t.Run("update (PUT) an identity", func(t *testing.T) {
		ctx.testContextChanged(t)
		enrolledId, _ := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false)
		enrolledIdentity := ctx.AdminManagementSession.requireQuery("identities/" + enrolledId)

		unenrolledId := ctx.AdminManagementSession.requireCreateIdentityOttEnrollmentUnfinished(eid.New(), false)
		unenrolledIdentity := ctx.AdminManagementSession.requireQuery("identities/" + unenrolledId)

		t.Run("should not alter authenticators", func(t *testing.T) {
			ctx.testContextChanged(t)
			updateContent := gabs.New()
			_, _ = updateContent.SetP(eid.New(), "name")
			_, _ = updateContent.SetP("Device", "type")
			_, _ = updateContent.SetP(map[string]interface{}{}, "tags")
			_, _ = updateContent.SetP(false, "isAdmin")

			resp := ctx.AdminManagementSession.updateEntityOfType(enrolledId, "identities", updateContent.String(), false)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			updatedIdentity := ctx.AdminManagementSession.requireQuery("identities/" + enrolledId)

			data := enrolledIdentity.Path("data.authenticators").Data()
			expectedAuths := data.(map[string]interface{})
			ctx.Req.NotEmpty(expectedAuths)

			updatedAuths := updatedIdentity.Path("data.authenticators").Data().(map[string]interface{})
			ctx.Req.NotEmpty(updatedAuths)
			ctx.Req.Equal(expectedAuths, updatedAuths)
		})

		t.Run("should not alter enrollments", func(t *testing.T) {
			ctx.testContextChanged(t)
			updateContent := gabs.New()
			_, _ = updateContent.SetP(eid.New(), "name")
			_, _ = updateContent.SetP("Device", "type")
			_, _ = updateContent.SetP(map[string]interface{}{}, "tags")
			_, _ = updateContent.SetP(false, "isAdmin")

			resp := ctx.AdminManagementSession.updateEntityOfType(unenrolledId, "identities", updateContent.String(), false)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			updatedIdentity := ctx.AdminManagementSession.requireQuery("identities/" + unenrolledId)

			expectedEnrollments := unenrolledIdentity.Path("data.enrollment").Data().(map[string]interface{})
			ctx.Req.NotEmpty(expectedEnrollments)

			updatedEnrollments := updatedIdentity.Path("data.enrollment").Data().(map[string]interface{})
			ctx.Req.NotEmpty(updatedEnrollments)

			ctx.Req.Equal(expectedEnrollments, updatedEnrollments)
		})

		t.Run("should not allow isDefaultAdmin to be altered", func(t *testing.T) {
			ctx.testContextChanged(t)
			identityId := ctx.AdminManagementSession.requireCreateIdentity(eid.New(), true)

			updateContent := gabs.New()
			_, _ = updateContent.SetP(eid.New(), "name")
			_, _ = updateContent.SetP("Device", "type")
			_, _ = updateContent.SetP(map[string]interface{}{}, "tags")
			_, _ = updateContent.SetP(true, "isAdmin")
			_, _ = updateContent.SetP(true, "isDefaultAdmin")

			resp := ctx.AdminManagementSession.updateEntityOfType(identityId, "identities", updateContent.String(), false)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			updatedIdentity := ctx.AdminManagementSession.requireQuery("identities/" + unenrolledId)

			ctx.Req.Equal(true, updatedIdentity.ExistsP("data.isDefaultAdmin"))
			isDefaultAdmin := updatedIdentity.Path("data.isDefaultAdmin").Data().(bool)
			ctx.Req.Equal(false, isDefaultAdmin)
		})

		t.Run("can update", func(t *testing.T) {
			ctx.testContextChanged(t)
			identityId := ctx.AdminManagementSession.requireCreateIdentity(eid.New(), true)

			newName := eid.New()
			updateContent := gabs.New()
			_, _ = updateContent.SetP(newName, "name")
			_, _ = updateContent.SetP("Device", "type")
			_, _ = updateContent.SetP(map[string]interface{}{}, "tags")
			_, _ = updateContent.SetP(false, "isAdmin")

			resp := ctx.AdminManagementSession.updateEntityOfType(identityId, "identities", updateContent.String(), false)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			updatedIdentity := ctx.AdminManagementSession.requireQuery("identities/" + identityId)

			t.Run("name", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Equal(true, updatedIdentity.ExistsP("data.name"))
				updatedName := updatedIdentity.Path("data.name").Data().(string)
				ctx.Req.Equal(newName, updatedName)
			})

			t.Run("isAdmin", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Equal(true, updatedIdentity.ExistsP("data.isAdmin"))
				newIsAdmin := updatedIdentity.Path("data.isAdmin").Data().(bool)
				ctx.Req.Equal(false, newIsAdmin)
			})
		})

	})

	t.Run("hasApiSessions", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityId, identityAuth := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment("identityHasApiSessionTest", false)

		t.Run("should be false if there are no API Sessions", func(t *testing.T) {
			ctx.testContextChanged(t)
			identityContainer := ctx.AdminManagementSession.requireQuery("/identities/" + identityId)

			ctx.Req.True(identityContainer.ExistsP("data.hasApiSession"), "expected field hasApiSession to exist")
			ctx.Req.False(identityContainer.Path("data.hasApiSession").Data().(bool), "expected hasApiSession to be false")
		})

		t.Run("should be true if there is 1 API Session", func(t *testing.T) {
			ctx.testContextChanged(t)

			session1, err := identityAuth.AuthenticateClientApi(ctx)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(session1)

			identityContainer := ctx.AdminManagementSession.requireQuery("/identities/" + identityId)
			ctx.Req.True(identityContainer.ExistsP("data.hasApiSession"), "expected field hasApiSession to exist")
			ctx.Req.True(identityContainer.Path("data.hasApiSession").Data().(bool), "expected hasApiSession to be true")

			t.Run("should be true if there is 1+ API Session", func(t *testing.T) {
				ctx.testContextChanged(t)

				session2, err := identityAuth.AuthenticateClientApi(ctx)
				ctx.Req.NoError(err)
				ctx.Req.NotNil(session2)

				identityContainer := ctx.AdminManagementSession.requireQuery("/identities/" + identityId)
				ctx.Req.True(identityContainer.ExistsP("data.hasApiSession"), "expected field hasApiSession to exist")
				ctx.Req.True(identityContainer.Path("data.hasApiSession").Data().(bool), "expected hasApiSession to be true")

				t.Run("should return to false after logouts", func(t *testing.T) {
					ctx.testContextChanged(t)

					_ = session1.logout()
					_ = session2.logout()

					identityContainer := ctx.AdminManagementSession.requireQuery("/identities/" + identityId)

					ctx.Req.True(identityContainer.ExistsP("data.hasApiSession"), "expected field hasApiSession to exist")
					ctx.Req.False(identityContainer.Path("data.hasApiSession").Data().(bool), "expected hasApiSession to be false")

				})
			})

		})

	})

}
