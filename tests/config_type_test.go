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
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/util/errorz"
	"math"
	"testing"
	"time"
)

func Test_ConfigTypes(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	identityRole := eid.New()
	nonAdminUser := ctx.AdminSession.createUserAndLogin(false, s(identityRole), nil)

	t.Run("create config type without name should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.newConfigType()
		configType.Name = ""
		resp := ctx.AdminSession.createEntity(configType)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "name")
	})

	t.Run("create config type without schema should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		now := time.Now()
		configType := ctx.AdminSession.requireCreateNewConfigType()

		entityJson := ctx.AdminSession.validateEntityWithQuery(configType)
		ctx.validateDateFieldsForCreate(now, entityJson)

		entityJson = ctx.AdminSession.validateEntityWithLookup(configType)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("create config type with invalid schema should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.newConfigType()
		configType.Schema = map[string]interface{}{
			"$id":                  "http://ziti-edge.netfoundry.io/schemas/test.config.json",
			"type":                 "objectionable", // not a valid type
			"additionalProperties": false,
			"required": []interface{}{
				"hostname",
				"port",
			},
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type": "string",
				},
			},
		}
		resp := ctx.AdminSession.createEntity(configType)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "schema")
	})

	t.Run("create config type with schema should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		now := time.Now()
		configType := ctx.newConfigType()
		configType.Schema = map[string]interface{}{
			"$id":                  "http://ziti-edge.netfoundry.io/schemas/test.config.json",
			"$schema":              "http://json-schema.org/draft-07/schema#",
			"type":                 "object",
			"additionalProperties": false,
			"required": []interface{}{
				"hostname",
				"port",
			},
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type": "string",
				},
				"port": map[string]interface{}{
					"type":    "integer",
					"minimum": float64(0),
					"maximum": float64(math.MaxUint16),
				},
			},
		}
		configType.Id = ctx.AdminSession.requireCreateEntity(configType)

		entityJson := ctx.AdminSession.validateEntityWithQuery(configType)
		ctx.validateDateFieldsForCreate(now, entityJson)

		entityJson = ctx.AdminSession.validateEntityWithLookup(configType)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("lookup non-existent config type as admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.RequireNotFoundError(ctx.AdminSession.query("config-types/" + eid.New()))
	})

	t.Run("lookup config type as non-admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		ctx.requireUnauthorizedError(nonAdminUser.query("config-types/" + configType.Id))
	})

	t.Run("update config type should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		now := time.Now()
		configType := ctx.AdminSession.requireCreateNewConfigType()

		entityJson := ctx.AdminSession.validateEntityWithQuery(configType)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		configType.Schema = map[string]interface{}{
			"$id":                  "http://ziti-edge.netfoundry.io/schemas/test.config.json",
			"type":                 "object",
			"additionalProperties": false,
			"required": []interface{}{
				"hostname",
				"port",
			},
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type": "string",
				},
				"port": map[string]interface{}{
					"type":    "integer",
					"minimum": float64(0),
					"maximum": float64(math.MaxUint16),
				},
			},
		}
		ctx.AdminSession.requireUpdateEntity(configType)
		jsonConfig := ctx.AdminSession.validateUpdate(configType)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonConfig)
	})

	t.Run("patch config should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		now := time.Now()
		configType := ctx.AdminSession.requireCreateNewConfigType()

		entityJson := ctx.AdminSession.validateEntityWithQuery(configType)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		configType.Schema = map[string]interface{}{
			"$id":                  "http://ziti-edge.netfoundry.io/schemas/test.config.json",
			"type":                 "object",
			"additionalProperties": false,
			"required": []interface{}{
				"hostname",
				"port",
			},
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type": "string",
				},
				"port": map[string]interface{}{
					"type":    "integer",
					"minimum": float64(0),
					"maximum": float64(math.MaxUint16),
				},
			},
		}
		ctx.AdminSession.requirePatchEntity(configType, "schema")
		jsonConfig := ctx.AdminSession.validateUpdate(configType)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonConfig)
	})

	t.Run("delete should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		now := time.Now()
		configType := ctx.AdminSession.requireCreateNewConfigType()

		entityJson := ctx.AdminSession.validateEntityWithQuery(configType)
		ctx.validateDateFieldsForCreate(now, entityJson)

		ctx.AdminSession.requireDeleteEntity(configType)
		ctx.RequireNotFoundError(ctx.AdminSession.query("config-types/" + configType.Id))
	})
}
