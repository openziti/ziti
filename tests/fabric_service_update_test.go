//go:build apitests

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
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
)

// Test_FabricServiceUpdatesPreserveEdgeFields verifies that updating an edge service through the
// fabric management API (PUT and PATCH) leaves the edge-owned fields (role attributes, configs,
// encryption required) undisturbed. After the fabric/edge service collapse both APIs write the
// same store, so the fabric surface must not clobber the fields it does not model.
func Test_FabricServiceUpdatesPreserveEdgeFields(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	configType := ctx.AdminManagementSession.requireCreateNewConfigType()
	cfg := ctx.AdminManagementSession.requireCreateNewConfig(configType.Id, map[string]interface{}{"port": float64(22)})

	// encryptionRequired=true so a fabric-side wipe to the zero value would be detected
	svc := ctx.newService(s("web"), s(cfg.Id))
	svc.encryptionRequired = true
	svc.Id = ctx.AdminManagementSession.requireCreateEntity(svc)

	// reading through the edge management API also proves the service is still an edge service:
	// a clobbered isFabricOnly would make this read 404
	assertEdgeFieldsIntact := func(expectedName string) {
		result := ctx.AdminManagementSession.requireQuery("services/" + svc.Id)
		data := ctx.RequireGetNonNilPathValue(result, "data")
		ctx.pathEquals(data, expectedName, path("name"))
		ctx.pathEqualsStringSlice(data, []string{"web"}, path("roleAttributes"))
		ctx.pathEqualsStringSlice(data, []string{cfg.Id}, path("configs"))
		ctx.pathEquals(data, true, path("encryptionRequired"))
	}
	assertEdgeFieldsIntact(svc.Name)

	// full update via fabric PUT, with only the fabric-owned fields in the body
	putName := eid.New()
	resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().
		SetBody(map[string]interface{}{
			"name":               putName,
			"terminatorStrategy": svc.terminatorStrategy,
		}).
		Put("https://localhost:1281/fabric/v1/services/" + svc.Id)
	ctx.Req.NoError(err)
	ctx.Req.True(resp.IsSuccess(), "fabric PUT failed: %v", string(resp.Body()))
	assertEdgeFieldsIntact(putName)

	// partial update via fabric PATCH
	patchName := eid.New()
	resp, err = ctx.AdminManagementSession.newAuthenticatedRequest().
		SetBody(map[string]interface{}{"name": patchName}).
		Patch("https://localhost:1281/fabric/v1/services/" + svc.Id)
	ctx.Req.NoError(err)
	ctx.Req.True(resp.IsSuccess(), "fabric PATCH failed: %v", string(resp.Body()))
	assertEdgeFieldsIntact(patchName)
}
