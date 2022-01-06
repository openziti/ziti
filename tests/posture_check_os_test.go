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
	"net/http"
	"testing"
)

func Test_PostureChecks_Os(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	t.Run("can CRUD OS posture checks", func(t *testing.T) {
		ctx.testContextChanged(t)

		originalName := uuid.New().String()
		originalTags := map[string]interface{}{
			"t1": "v1",
		}

		originalOses := []map[string]interface{}{
			{
				"type":     "Windows",
				"versions": []interface{}{">10.0.0"},
			},
		}

		originalTypeId := "OS"

		t.Run("can create", func(t *testing.T) {
			ctx.testContextChanged(t)

			osPost := gabs.New()
			_, _ = osPost.Set(originalName, "name")
			_, _ = osPost.Set(originalTags, "tags")
			_, _ = osPost.Set(originalTypeId, "typeId")
			_, _ = osPost.Set(originalOses, "operatingSystems")

			resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(osPost.String()).Post("/posture-checks")
			ctx.Req.NoError(err)
			ctx.Req.Equal(http.StatusCreated, resp.StatusCode())

			createdPostureCheck, err := gabs.ParseJSON(resp.Body())
			ctx.Req.NoError(err)
			ctx.Req.True(createdPostureCheck.ExistsP("data.id"))
			postureCheckId := createdPostureCheck.Path("data.id").Data().(string)
			ctx.Req.NotEmpty(postureCheckId)

			t.Run("can get", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(osPost.String()).Get("/posture-checks/" + postureCheckId)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, resp.StatusCode())

				checkContainer, err := gabs.ParseJSON(resp.Body())
				ctx.Req.NoError(err)
				ctx.Req.NotNil(checkContainer)

				ctx.Req.True(checkContainer.ExistsP("data.name"), "should have a name")
				ctx.Req.Equal(originalName, checkContainer.Path("data.name").Data().(string))

				ctx.Req.True(checkContainer.ExistsP("data.tags"), "should have tags")
				ctx.Req.Equal(originalTags, checkContainer.Path("data.tags").Data().(map[string]interface{}))

				ctx.Req.True(checkContainer.ExistsP("data.typeId"), "should have a typeId")
				ctx.Req.Equal(originalTypeId, checkContainer.Path("data.typeId").Data().(string))

				ctx.Req.True(checkContainer.ExistsP("data.operatingSystems"), "should have an OS array")
				oses, err := checkContainer.Path("data.operatingSystems").Children()
				ctx.Req.NoError(err)
				ctx.Req.Len(oses, 1, "should have 1 os")

				ctx.Req.True(oses[0].ExistsP("versions"), "should have os versions")
				ctx.Req.True(oses[0].ExistsP("type"), "should have an os type")
				ctx.Req.Equal(originalOses[0], oses[0].Data())
			})

			newOses := []map[string]interface{}{
				{
					"type":     "Linux",
					"versions": []interface{}{">9.0.0"},
				},
			}

			t.Run("can patch oses", func(t *testing.T) {
				ctx.testContextChanged(t)

				osPatch := gabs.New()
				_, _ = osPatch.Set(newOses, "operatingSystems")
				_, _ = osPatch.Set(originalTypeId, "typeId")

				patchResp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(osPatch.String()).Patch("/posture-checks/" + postureCheckId)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, patchResp.StatusCode())

				t.Run("get after os patch has proper values", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(osPost.String()).Get("/posture-checks/" + postureCheckId)
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

					checkContainer, err := gabs.ParseJSON(resp.Body())
					ctx.Req.NoError(err)
					ctx.Req.NotNil(checkContainer)

					ctx.Req.True(checkContainer.ExistsP("data.name"), "should have a name")
					ctx.Req.Equal(originalName, checkContainer.Path("data.name").Data().(string))

					ctx.Req.True(checkContainer.ExistsP("data.tags"), "should have tags")
					ctx.Req.Equal(originalTags, checkContainer.Path("data.tags").Data().(map[string]interface{}))

					ctx.Req.True(checkContainer.ExistsP("data.typeId"), "should have a typeId")
					ctx.Req.Equal(originalTypeId, checkContainer.Path("data.typeId").Data().(string))

					ctx.Req.True(checkContainer.ExistsP("data.operatingSystems"), "should have an OS array")
					oses, err := checkContainer.Path("data.operatingSystems").Children()
					ctx.Req.NoError(err)
					ctx.Req.Len(oses, 1, "should have 1 os")

					ctx.Req.True(oses[0].ExistsP("versions"), "should have os versions")
					ctx.Req.True(oses[0].ExistsP("type"), "should have an os type")
					ctx.Req.Equal(newOses[0], oses[0].Data())
				})
			})

			t.Run("can patch tags", func(t *testing.T) {
				ctx.testContextChanged(t)

				newTags := map[string]interface{}{
					"t2": "v2",
				}

				osPatch := gabs.New()
				_, _ = osPatch.Set(newTags, "tags")
				_, _ = osPatch.Set(originalTypeId, "typeId")

				patchResp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(osPatch.String()).Patch("/posture-checks/" + postureCheckId)
				ctx.Req.NoError(err)
				ctx.Req.Equal(http.StatusOK, patchResp.StatusCode())

				t.Run("get after tags patch has proper values", func(t *testing.T) {
					ctx.testContextChanged(t)

					resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().SetBody(osPost.String()).Get("/posture-checks/" + postureCheckId)
					ctx.Req.NoError(err)
					ctx.Req.Equal(http.StatusOK, resp.StatusCode())

					checkContainer, err := gabs.ParseJSON(resp.Body())
					ctx.Req.NoError(err)
					ctx.Req.NotNil(checkContainer)

					ctx.Req.True(checkContainer.ExistsP("data.name"), "should have a name")
					ctx.Req.Equal(originalName, checkContainer.Path("data.name").Data().(string))

					ctx.Req.True(checkContainer.ExistsP("data.tags"), "should have tags")
					ctx.Req.Equal(newTags, checkContainer.Path("data.tags").Data().(map[string]interface{}))

					ctx.Req.True(checkContainer.ExistsP("data.typeId"), "should have a typeId")
					ctx.Req.Equal(originalTypeId, checkContainer.Path("data.typeId").Data().(string))

					ctx.Req.True(checkContainer.ExistsP("data.operatingSystems"), "should have an OS array")
					oses, err := checkContainer.Path("data.operatingSystems").Children()
					ctx.Req.NoError(err)
					ctx.Req.Len(oses, 1, "should have 1 os")

					ctx.Req.True(oses[0].ExistsP("versions"), "should have os versions")
					ctx.Req.True(oses[0].ExistsP("type"), "should have an os type")
					ctx.Req.Equal(newOses[0], oses[0].Data()) //newOses from patch
				})
			})

			t.Run("can delete", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.AdminManagementSession.deleteEntityOfType("posture-checks", postureCheckId)
			})
		})

	})
}
