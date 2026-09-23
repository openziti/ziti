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

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/config/routerlink"
)

// Semantic validation of router.link.v1 has to happen where the operator wrote
// the config. A JSON schema can't express a relationship between two fields, so
// without this the controller accepts a config that every router then rejects,
// and the only sign is a line in each router's log.
func Test_RouterLinkConfigValidation(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	heartbeats := func(fields map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"heartbeats": fields}
	}

	t.Run("accepts sane heartbeat timing", func(t *testing.T) {
		ctx.testContextChanged(t)
		config := ctx.newConfig(routerlink.ConfigTypeV1, heartbeats(map[string]interface{}{
			"sendInterval":             "10s",
			"checkInterval":            "1s",
			"closeUnresponsiveTimeout": "60s",
		}))
		config.Id = ctx.AdminManagementSession.requireCreateEntity(config)
		ctx.Req.NotEmpty(config.Id)
	})

	t.Run("rejects a check interval past the timeout", func(t *testing.T) {
		ctx.testContextChanged(t)
		config := ctx.newConfig(routerlink.ConfigTypeV1, heartbeats(map[string]interface{}{
			"checkInterval":            "2m",
			"closeUnresponsiveTimeout": "30s",
		}))
		resp := ctx.AdminManagementSession.createEntity(config)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "data")
	})

	t.Run("rejects a non-positive interval", func(t *testing.T) {
		ctx.testContextChanged(t)
		config := ctx.newConfig(routerlink.ConfigTypeV1, heartbeats(map[string]interface{}{
			"checkInterval": "0s",
		}))
		resp := ctx.AdminManagementSession.createEntity(config)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "data")
	})

	t.Run("rejects a timeout under the default send interval", func(t *testing.T) {
		// The config names no intervals, so validation has to resolve the
		// defaults to see that this combination is broken.
		ctx.testContextChanged(t)
		config := ctx.newConfig(routerlink.ConfigTypeV1, heartbeats(map[string]interface{}{
			"closeUnresponsiveTimeout": "5s",
		}))
		resp := ctx.AdminManagementSession.createEntity(config)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "data")
	})

	t.Run("rejects an unsafe update to a valid config", func(t *testing.T) {
		ctx.testContextChanged(t)
		config := ctx.newConfig(routerlink.ConfigTypeV1, heartbeats(map[string]interface{}{
			"closeUnresponsiveTimeout": "60s",
		}))
		config.Id = ctx.AdminManagementSession.requireCreateEntity(config)

		// Validation runs on update as well as create, so a config can't be
		// walked into an unsafe state after the fact.
		config.Data = heartbeats(map[string]interface{}{"closeUnresponsiveTimeout": "1s"})
		resp := ctx.AdminManagementSession.updateEntity(config)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "data")
	})
}
