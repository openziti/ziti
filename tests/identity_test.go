//go:build apitests
// +build apitests

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
	"net/http"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_model"
	nfpem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/db"
)

func Test_Identity(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("a new identity with an ott enrollment can be created", func(t *testing.T) {
		ctx.testContextChanged(t)

		hostCost := rest_model.TerminatorCost(5)
		hostPrecedence := rest_model.TerminatorPrecedenceDefault

		identityType := rest_model.IdentityTypeUser

		identityCreate := &rest_model.IdentityCreate{
			AppData: &rest_model.Tags{
				SubTags: map[string]any{
					"one": 1,
					"two": 2,
				},
			},
			AuthPolicyID:             S("default"),
			DefaultHostingCost:       &hostCost,
			DefaultHostingPrecedence: hostPrecedence,
			Enrollment: &rest_model.IdentityCreateEnrollment{
				Ott: true,
			},
			ExternalID:     S(uuid.NewString()),
			IsAdmin:        B(false),
			Name:           S(uuid.NewString()),
			RoleAttributes: &rest_model.Attributes{"one", "two"},
			Tags: &rest_model.Tags{
				SubTags: map[string]any{
					"one": 1,
					"two": 2,
				},
			},
			Type: &identityType,
		}

		err := identityCreate.Validate(DefaultFormats)
		ctx.Req.NoError(err)

		identityCreateResp := &rest_model.CreateEnvelope{}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(identityCreateResp).SetBody(identityCreate).Post("/identities")
		ctx.NoError(err)
		ctx.NotNil(resp)
		ctx.Equal(201, resp.StatusCode())

		ctx.NoError(identityCreateResp.Validate(DefaultFormats))

		enrollmentResp := &rest_model.ListEnrollmentsEnvelope{}
		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(enrollmentResp).Get("/identities/" + identityCreateResp.Data.ID + "/enrollments")
		ctx.NoError(err)
		ctx.NotNil(resp)
		ctx.Equal(http.StatusOK, resp.StatusCode(), string(resp.Body()))
		ctx.NoError(enrollmentResp.Validate(DefaultFormats), string(resp.Body()))
		ctx.NotNil(enrollmentResp.Data)
		ctx.Len(enrollmentResp.Data, 1)
	})

	t.Run("role attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		identity := newTestIdentity(false, role1, role2)
		identity.Id = ctx.AdminManagementSession.requireCreateEntity(identity)
		ctx.AdminManagementSession.validateEntityWithQuery(identity)
		ctx.AdminManagementSession.validateEntityWithLookup(identity)
	})

	t.Run("auth policy should default", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityType := rest_model.IdentityTypeUser
		identityCreate := &rest_model.IdentityCreate{
			Type:    &identityType,
			Name:    S("identity-name-default-auth-policy"),
			IsAdmin: B(false),
		}

		createResponse := &rest_model.CreateEnvelope{}
		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityCreate).SetResult(createResponse).Post("/identities")

		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 CREATED")
		ctx.Req.NotEmpty(createResponse.Data.ID)

		getResponse := &rest_model.DetailIdentityEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(getResponse).Get("/identities/" + createResponse.Data.ID)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())
		ctx.Req.Equal(db.DefaultAuthPolicyId, *getResponse.Data.AuthPolicyID)

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

		identity.roleAttributes = []string{}
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

	t.Run("can patch is admin to false", func(t *testing.T) {
		ctx.testContextChanged(t)
		identity := newTestIdentity(true)
		identity.Id = ctx.AdminManagementSession.requireCreateEntity(identity)

		identityPatch := &rest_model.IdentityPatch{
			IsAdmin: B(false),
		}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityPatch).Patch("/identities/" + identity.Id)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		t.Run("has the proper values set", func(t *testing.T) {
			ctx.testContextChanged(t)

			identityDetail := &rest_model.DetailIdentityEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(identityDetail).Get("/identities/" + identity.Id)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			ctx.Req.Equal(identity.name, *identityDetail.Data.Name)
			ctx.Req.Equal(identity.identityType, *identityDetail.Data.TypeID)
			ctx.Req.Equal(*identityPatch.IsAdmin, *identityDetail.Data.IsAdmin)
		})
	})

	t.Run("can patch is admin to true", func(t *testing.T) {
		ctx.testContextChanged(t)
		identity := newTestIdentity(false)
		identity.Id = ctx.AdminManagementSession.requireCreateEntity(identity)

		identityPatch := &rest_model.IdentityPatch{
			IsAdmin: B(true),
		}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityPatch).Patch("/identities/" + identity.Id)
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusOK, resp.StatusCode())

		t.Run("has the proper values set", func(t *testing.T) {
			ctx.testContextChanged(t)

			identityDetail := &rest_model.DetailIdentityEnvelope{}

			resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(identityDetail).Get("/identities/" + identity.Id)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			ctx.Req.Equal(identity.name, *identityDetail.Data.Name)
			ctx.Req.Equal(identity.identityType, *identityDetail.Data.TypeID)
			ctx.Req.Equal(*identityPatch.IsAdmin, *identityDetail.Data.IsAdmin)
		})
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

	t.Run("can create identity", func(t *testing.T) {
		ctx.testContextChanged(t)

		terminatorCost := rest_model.TerminatorCost(1)
		identityType := rest_model.IdentityTypeDefault

		identityCreate := &rest_model.IdentityCreate{
			AppData: &rest_model.Tags{
				SubTags: map[string]interface{}{
					"key1": "value1",
				},
			},
			AuthPolicyID:             S("default"),
			DefaultHostingCost:       &terminatorCost,
			DefaultHostingPrecedence: rest_model.TerminatorPrecedenceFailed,
			Enrollment: &rest_model.IdentityCreateEnrollment{
				Ott: true,
			},
			ExternalID:     S("hello-there-external-id"),
			IsAdmin:        B(true),
			Name:           S("hello-there-name"),
			RoleAttributes: &rest_model.Attributes{"attribute1"},
			Tags: &rest_model.Tags{
				SubTags: map[string]interface{}{
					"key1": "value1",
				},
			},
			Type: &identityType,
		}

		identityCreateResult := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identityCreate).SetResult(identityCreateResult).Post("/identities")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
		ctx.Req.NotEmpty(identityCreateResult.Data.ID)

		t.Run("get identity values match create value", func(t *testing.T) {
			ctx.testContextChanged(t)

			identityDetailResult := &rest_model.CurrentIdentityDetailEnvelope{}
			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetResult(identityDetailResult).Get("/identities/" + identityCreateResult.Data.ID)
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode(), "response body: %s", resp.Body())
			ctx.Req.NotNil(identityDetailResult.Data)

			identityDetail := identityDetailResult.Data

			ctx.Req.Equal(identityCreate.AppData.SubTags["key1"], identityDetail.AppData.SubTags["key1"])
			ctx.Req.Equal(*identityCreate.AuthPolicyID, *identityDetail.AuthPolicyID)
			ctx.Req.Equal(*identityCreate.DefaultHostingCost, *identityDetail.DefaultHostingCost)
			ctx.Req.Equal(identityCreate.DefaultHostingPrecedence, identityDetail.DefaultHostingPrecedence)
			ctx.Req.NotNil(identityDetail.Enrollment.Ott)
			ctx.Req.Nil(identityDetail.Enrollment.Updb)
			ctx.Req.Nil(identityDetail.Enrollment.Ottca)
			ctx.Req.Equal(*identityCreate.ExternalID, *identityDetail.ExternalID)
			ctx.Req.Equal(*identityCreate.IsAdmin, *identityDetail.IsAdmin)
			ctx.Req.Equal(*identityCreate.Name, *identityDetail.Name)
			ctx.Req.Equal((*identityCreate.RoleAttributes)[0], (*identityDetail.RoleAttributes)[0])
			ctx.Req.Equal(identityCreate.Tags.SubTags["key1"], identityDetail.Tags.SubTags["key1"])
			ctx.Req.NotNil(identityDetail.Type)
			ctx.Req.Equal(string(*identityCreate.Type), identityDetail.Type.Name)
		})
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
			_, _ = updateContent.SetP(rest_model.IdentityTypeDefault, "type")
			_, _ = updateContent.SetP(map[string]interface{}{}, "tags")
			_, _ = updateContent.SetP(false, "isAdmin")
			_, _ = updateContent.SetP("", "authPolicyId")

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
			_, _ = updateContent.SetP(rest_model.IdentityTypeDefault, "type")
			_, _ = updateContent.SetP(rest_model.IdentityTypeDefault, "type")
			_, _ = updateContent.SetP(map[string]interface{}{}, "tags")
			_, _ = updateContent.SetP(false, "isAdmin")
			_, _ = updateContent.SetP("", "authPolicyId")

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
			_, _ = updateContent.SetP(rest_model.IdentityTypeDefault, "type")
			_, _ = updateContent.SetP(map[string]interface{}{}, "tags")
			_, _ = updateContent.SetP(true, "isAdmin")
			_, _ = updateContent.SetP(true, "isDefaultAdmin")
			_, _ = updateContent.SetP("", "authPolicyId")

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
			_, _ = updateContent.SetP(rest_model.IdentityTypeDefault, "type")
			_, _ = updateContent.SetP(map[string]interface{}{}, "tags")
			_, _ = updateContent.SetP(false, "isAdmin")
			_, _ = updateContent.SetP("", "authPolicyId")
			_, _ = updateContent.SetP("new-external-id", "externalId")

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

			t.Run("externalId", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.Equal(true, updatedIdentity.ExistsP("data.externalId"))
				newExternalId := updatedIdentity.Path("data.externalId").Data().(string)
				ctx.Req.Equal("new-external-id", newExternalId)
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

	t.Run("disable and enable identities affect cert authentication", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityType := rest_model.IdentityTypeDefault
		identity := &rest_model.IdentityCreate{
			IsAdmin: B(false),
			Name:    S("test-identity-disable-cert"),
			Type:    &identityType,
			Enrollment: &rest_model.IdentityCreateEnrollment{
				Ott: true,
			},
		}

		identityCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identity).SetResult(identityCreated).Post("/identities")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", identity, resp.Body())
		ctx.Req.NotEmpty(identityCreated.Data.ID)

		certAuthenticator := ctx.completeOttEnrollment(identityCreated.Data.ID)

		t.Run("identity can authenticate", func(t *testing.T) {
			ctx.testContextChanged(t)
			apiSession, err := certAuthenticator.AuthenticateClientApi(ctx)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(apiSession)
			ctx.Req.NotEmpty(apiSession.AuthResponse.Token)

			t.Run("identity can be disabled", func(t *testing.T) {
				ctx.testContextChanged(t)

				disable := &rest_model.DisableParams{
					DurationMinutes: I(0),
				}
				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(disable).Post("identities/" + identityCreated.Data.ID + "/disable")
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				t.Run("identity api session removed", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := apiSession.newAuthenticatedRequest().Get("current-api-session")
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
				})

				t.Run("identity cannot authenticate", func(t *testing.T) {
					ctx.testContextChanged(t)
					apiSession, err := certAuthenticator.AuthenticateClientApi(ctx)
					ctx.Req.Error(err)
					ctx.Req.Nil(apiSession)
				})

				t.Run("identity can be enabled", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(disable).Post("identities/" + identityCreated.Data.ID + "/enable")
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

					t.Run("identity can authenticate", func(t *testing.T) {
						ctx.testContextChanged(t)
						apiSession, err := certAuthenticator.AuthenticateClientApi(ctx)
						ctx.Req.NoError(err)
						ctx.Req.NotNil(apiSession)
					})

				})
			})
		})
	})

	t.Run("disable and enable identities affect updb authentication", func(t *testing.T) {
		ctx.testContextChanged(t)
		username := "test-identity-disable-updb"
		password := "test-identity-disable-updb-password"
		identityType := rest_model.IdentityTypeDefault
		identity := &rest_model.IdentityCreate{
			IsAdmin: B(false),
			Name:    S("test-identity-disable-updb"),
			Type:    &identityType,
			Enrollment: &rest_model.IdentityCreateEnrollment{
				Updb: username,
			},
		}

		identityCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identity).SetResult(identityCreated).Post("/identities")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", identity, resp.Body())
		ctx.Req.NotEmpty(identityCreated.Data.ID)

		ctx.completeUpdbEnrollment(identityCreated.Data.ID, password)

		authenticator := &updbAuthenticator{
			Username: username,
			Password: password,
		}

		t.Run("identity can authenticate", func(t *testing.T) {
			ctx.testContextChanged(t)

			apiSession, err := authenticator.AuthenticateClientApi(ctx)
			ctx.Req.NoError(err)
			ctx.Req.NotNil(apiSession)

			t.Run("identity can be disabled", func(t *testing.T) {
				ctx.testContextChanged(t)

				disable := &rest_model.DisableParams{
					DurationMinutes: I(0),
				}
				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(disable).Post("identities/" + identityCreated.Data.ID + "/disable")
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				t.Run("identity api session removed", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := apiSession.newAuthenticatedRequest().Get("current-api-session")
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
				})

				t.Run("identity cannot authenticate", func(t *testing.T) {
					ctx.testContextChanged(t)
					apiSession, err := authenticator.AuthenticateClientApi(ctx)
					ctx.Req.Error(err)
					ctx.Req.Nil(apiSession)
				})

				t.Run("identity can be enabled", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(disable).Post("identities/" + identityCreated.Data.ID + "/enable")
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

					t.Run("identity can authenticate", func(t *testing.T) {
						ctx.testContextChanged(t)
						apiSession, err := authenticator.AuthenticateClientApi(ctx)
						ctx.Req.NoError(err)
						ctx.Req.NotNil(apiSession)
					})

				})
			})
		})
	})

	t.Run("disable and enable identities affect ext jwt authentication", func(t *testing.T) {
		ctx.testContextChanged(t)

		jwtSignerCert, jwtSignerPrivate := newSelfSignedCert("Test Jwt Signer Cert - Identity Disabled Test 01")

		extJwtSigner := &rest_model.ExternalJWTSignerCreate{
			CertPem:  S(nfpem.EncodeToString(jwtSignerCert)),
			Enabled:  B(true),
			Name:     S("Test JWT Signer - Auth Policy - Identity Disable Test 01"),
			Kid:      S(uuid.NewString()),
			Issuer:   S(uuid.NewString()),
			Audience: S(uuid.NewString()),
		}

		extJwtSignerCreated := &rest_model.CreateEnvelope{}

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(extJwtSigner).SetResult(extJwtSignerCreated).Post("/external-jwt-signers")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", extJwtSigner, resp.Body())
		ctx.Req.NotEmpty(extJwtSignerCreated.Data.ID)

		authPolicyPatch := &rest_model.AuthPolicyPatch{
			Primary: &rest_model.AuthPolicyPrimaryPatch{
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
					AllowedSigners: []string{extJwtSignerCreated.Data.ID},
				},
			},
		}
		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(authPolicyPatch).Patch("/auth-policies/default")
		ctx.NoError(err)
		ctx.Equal(http.StatusOK, resp.StatusCode())

		identityType := rest_model.IdentityTypeDefault
		identity := &rest_model.IdentityCreate{
			IsAdmin: B(false),
			Name:    S("test-identity-disable-updb-01"),
			Type:    &identityType,
		}

		identityCreated := &rest_model.CreateEnvelope{}

		resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(identity).SetResult(identityCreated).Post("/identities")
		ctx.Req.NoError(err)
		ctx.Req.Equal(http.StatusCreated, resp.StatusCode(), "expected 201 for POST %T: %s", identity, resp.Body())
		ctx.Req.NotEmpty(identityCreated.Data.ID)

		t.Run("identity can authenticate", func(t *testing.T) {
			ctx.testContextChanged(t)

			jwtToken := jwt.New(jwt.SigningMethodES256)
			jwtToken.Claims = jwt.RegisteredClaims{
				Audience:  []string{*extJwtSigner.Audience},
				ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(2 * time.Hour)},
				ID:        time.Now().String(),
				IssuedAt:  &jwt.NumericDate{Time: time.Now()},
				Issuer:    *extJwtSigner.Issuer,
				NotBefore: &jwt.NumericDate{Time: time.Now()},
				Subject:   identityCreated.Data.ID,
			}

			jwtToken.Header["kid"] = *extJwtSigner.Kid

			jwtStrSigned, err := jwtToken.SignedString(jwtSignerPrivate)
			ctx.Req.NoError(err)
			ctx.Req.NotEmpty(jwtStrSigned)

			result := &rest_model.CurrentAPISessionDetailEnvelope{}

			resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusOK, resp.StatusCode())

			ctx.Req.NotNil(result)
			ctx.Req.NotNil(result.Data)
			ctx.Req.NotNil(result.Data.Token)

			t.Run("identity can be disabled", func(t *testing.T) {
				ctx.testContextChanged(t)

				disable := &rest_model.DisableParams{
					DurationMinutes: I(0),
				}
				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(disable).Post("identities/" + identityCreated.Data.ID + "/disable")
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				t.Run("identity api session removed", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := ctx.newAnonymousClientApiRequest().SetHeader("Authorization", "Bearer "+jwtStrSigned).Get("current-api-session")
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
				})

				t.Run("identity cannot authenticate", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusUnauthorized, resp.StatusCode())
				})

				t.Run("identity can be enabled", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(disable).Post("identities/" + identityCreated.Data.ID + "/enable")
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

					t.Run("identity can authenticate", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := ctx.newAnonymousClientApiRequest().SetResult(result).SetHeader("Authorization", "Bearer "+jwtStrSigned).Post("/authenticate?method=ext-jwt")
						ctx.Req.NoError(err)
						ctx.Req.Equal(http.StatusOK, resp.StatusCode())
					})

				})
			})
		})
	})

}
