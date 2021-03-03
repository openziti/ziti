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
	"net/http"
	"testing"
	"time"
)

func Test_Configs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	identityRole := eid.New()
	nonAdminUser := ctx.AdminSession.createUserAndLogin(false, s(identityRole), nil)

	t.Run("create without name should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		config := ctx.newConfig(configType.Id, map[string]interface{}{"port": 22})
		config.Name = ""
		resp := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "name")
	})

	t.Run("create without data should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		config := ctx.newConfig(configType.Id, nil)
		resp := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "data")
	})

	t.Run("create without type should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		config := ctx.newConfig("", map[string]interface{}{"port": 22})
		resp := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "type")
	})

	t.Run("create with invalid config type should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		config := ctx.newConfig(eid.New(), map[string]interface{}{"port": 22})
		resp := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "type")
	})

	t.Run("create should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()

		now := time.Now()
		config := ctx.AdminSession.requireCreateNewConfig(configType.Id, map[string]interface{}{"port": float64(22)})
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		entityJson = ctx.AdminSession.validateEntityWithLookup(config)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("create with nested values should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		now := time.Now()
		config := ctx.newConfig(configType.Id, map[string]interface{}{
			"port":     float64(22),
			"hostname": "ssh.globotech.bizniz",
			"enabled":  true,
			"nested": map[string]interface{}{
				"neato":   false,
				"count":   123.32,
				"scrappy": "appley",
				"nesting weird names ?": map[string]interface{}{
					"valuable":     false,
					"questionable": true,
					"Snoodgrass":   float64(321321312),
				},
			},
		})
		config.Tags = map[string]interface{}{"org": "na", "deprecated": false}
		config.Id = ctx.AdminSession.requireCreateEntity(config)
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		entityJson = ctx.AdminSession.validateEntityWithLookup(config)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("lookup non-existent config as admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.RequireNotFoundError(ctx.AdminSession.query("configs/" + eid.New()))
	})

	t.Run("lookup config as non-admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		config := ctx.AdminSession.requireCreateNewConfig(configType.Id, map[string]interface{}{"port": 22})
		ctx.requireUnauthorizedError(nonAdminUser.query("configs/" + config.Id))
	})

	t.Run("update config should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()

		now := time.Now()
		config := ctx.AdminSession.requireCreateNewConfig(configType.Id, map[string]interface{}{"port": float64(22)})
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.Data = map[string]interface{}{"snafu": false}
		ctx.AdminSession.requireUpdateEntity(config)
		jsonConfig := ctx.AdminSession.validateUpdate(config)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonConfig)
	})

	t.Run("patch config should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		now := time.Now()
		config := ctx.AdminSession.requireCreateNewConfig(configType.Id, map[string]interface{}{"port": float64(22)})
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		newName := eid.New()
		config.Name = newName
		config.Data = map[string]interface{}{"foo": "bar"}
		config.Tags = map[string]interface{}{"baz": "bam"}
		ctx.AdminSession.requirePatchEntity(config, "name")

		config.Data = map[string]interface{}{"port": float64(22)} // data should not have gotten updated
		config.Tags = nil                                         // tags should not be updated
		jsonConfig := ctx.AdminSession.validateUpdate(config)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonConfig)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.Name = eid.New()
		config.Data = map[string]interface{}{"foo": "bar"}
		config.Tags = map[string]interface{}{"baz": "bam"}
		ctx.AdminSession.requirePatchEntity(config, "data")

		config.Name = newName // name should not be updated
		config.Tags = nil     // tags should not be updated
		ctx.AdminSession.validateUpdate(config)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.Name = eid.New()
		config.Data = map[string]interface{}{"bim": "bam"}
		config.Tags = map[string]interface{}{"enlightened": false}
		ctx.AdminSession.requirePatchEntity(config, "tags")

		config.Name = newName                              // name should not be updated
		config.Data = map[string]interface{}{"foo": "bar"} // data should not have gotten updated
		ctx.AdminSession.validateUpdate(config)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.Name = eid.New()
		config.Data = map[string]interface{}{"bim": "bom"}
		config.Tags = map[string]interface{}{"enlightened": true}
		ctx.AdminSession.requirePatchEntity(config, "name", "data", "tags")
		ctx.AdminSession.validateUpdate(config)
	})

	t.Run("update of type config should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		config := ctx.AdminSession.requireCreateNewConfig(configType.Id, map[string]interface{}{"port": float64(22)})
		ctx.AdminSession.validateEntityWithQuery(config)

		configType2 := ctx.AdminSession.requireCreateNewConfigType()
		config.sendType = false
		config.ConfigTypeId = configType2.Id
		ctx.AdminSession.requireUpdateEntity(config)

		config.ConfigTypeId = configType.Id
		ctx.AdminSession.validateEntityWithQuery(config)

		config.ConfigTypeId = configType2.Id
		ctx.AdminSession.requirePatchEntity(config, "name", "type")

		config.ConfigTypeId = configType.Id
		ctx.AdminSession.validateEntityWithQuery(config)
	})

	t.Run("delete should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		now := time.Now()
		config := ctx.AdminSession.requireCreateNewConfig(configType.Id, map[string]interface{}{"port": float64(22)})
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		ctx.AdminSession.requireDeleteEntity(config)
		ctx.RequireNotFoundError(ctx.AdminSession.query("configs/" + config.Id))
	})

	t.Run("create config type with non-object schema should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.testContextChanged(t)
		resp := ctx.AdminSession.createEntityOfType("config-types", map[string]interface{}{
			"name":   eid.New(),
			"schema": "not-object",
		})
		standardErrorJsonResponseTests(resp, errorz.CouldNotValidateCode, http.StatusBadRequest, t)
	})

	t.Run("create config type with schema should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.newConfigType()
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
		configType.Id = ctx.AdminSession.requireCreateEntity(configType)

		config := ctx.newConfig(configType.Id, map[string]interface{}{"port": 22})
		resp := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), errorz.CouldNotValidateCode, "(root)")

		now := time.Now()
		config = ctx.newConfig(configType.Id, map[string]interface{}{
			"hostname": "ssh.globotech.bizniz",
			"port":     float64(22),
		})
		config.Id = ctx.AdminSession.requireCreateEntity(config)
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})
}
