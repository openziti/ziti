// +build apitests

/*
	Copyright 2020 Netfoundry, Inc.

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
	"math"
	"testing"
	"time"

	"github.com/netfoundry/ziti-edge/controller/apierror"

	"github.com/google/uuid"
)

func Test_Configs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	identityRole := uuid.New().String()
	nonAdminUser := ctx.AdminSession.createUserAndLogin(false, s(identityRole), nil)

	t.Run("create without name should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		config := ctx.newConfig(configType.id, map[string]interface{}{"port": 22})
		config.name = ""
		httpCode, body := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "name")
	})

	t.Run("create without data should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		config := ctx.newConfig(configType.id, nil)
		httpCode, body := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "data")
	})

	t.Run("create without type should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		config := ctx.newConfig("", map[string]interface{}{"port": 22})
		httpCode, body := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "type")
	})

	t.Run("create with invalid config type should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		config := ctx.newConfig(uuid.New().String(), map[string]interface{}{"port": 22})
		httpCode, body := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(httpCode, body, apierror.InvalidFieldCode, "type")
	})

	t.Run("create should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()

		now := time.Now()
		config := ctx.AdminSession.requireCreateNewConfig(configType.id, map[string]interface{}{"port": float64(22)})
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		entityJson = ctx.AdminSession.validateEntityWithLookup(config)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("create using config name should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()

		now := time.Now()
		config := ctx.AdminSession.requireCreateNewConfig(configType.name, map[string]interface{}{"port": float64(22)})
		config.configType = configType.id
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		entityJson = ctx.AdminSession.validateEntityWithLookup(config)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("create with nested values should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		now := time.Now()
		config := ctx.newConfig(configType.id, map[string]interface{}{
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
		config.tags = map[string]interface{}{"org": "na", "deprecated": false}
		config.id = ctx.AdminSession.requireCreateEntity(config)
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		entityJson = ctx.AdminSession.validateEntityWithLookup(config)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("lookup non-existent config as admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.requireNotFoundError(ctx.AdminSession.query("configs/" + uuid.New().String()))
	})

	t.Run("lookup config as non-admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		config := ctx.AdminSession.requireCreateNewConfig(configType.id, map[string]interface{}{"port": 22})
		ctx.requireUnauthorizedError(nonAdminUser.query("configs/" + config.id))
	})

	t.Run("update config should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()

		now := time.Now()
		config := ctx.AdminSession.requireCreateNewConfig(configType.id, map[string]interface{}{"port": float64(22)})
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.data = map[string]interface{}{"snafu": false}
		ctx.AdminSession.requireUpdateEntity(config)
		jsonConfig := ctx.AdminSession.validateUpdate(config)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonConfig)
	})

	t.Run("patch config should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		now := time.Now()
		config := ctx.AdminSession.requireCreateNewConfig(configType.id, map[string]interface{}{"port": float64(22)})
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		newName := uuid.New().String()
		config.name = newName
		config.data = map[string]interface{}{"foo": "bar"}
		config.tags = map[string]interface{}{"baz": "bam"}
		ctx.AdminSession.requirePatchEntity(config, "name")

		config.data = map[string]interface{}{"port": float64(22)} // data should not have gotten updated
		config.tags = nil                                         // tags should not be updated
		jsonConfig := ctx.AdminSession.validateUpdate(config)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonConfig)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.name = uuid.New().String()
		config.data = map[string]interface{}{"foo": "bar"}
		config.tags = map[string]interface{}{"baz": "bam"}
		ctx.AdminSession.requirePatchEntity(config, "data")

		config.name = newName // name should not be updated
		config.tags = nil     // tags should not be updated
		ctx.AdminSession.validateUpdate(config)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.name = uuid.New().String()
		config.data = map[string]interface{}{"bim": "bam"}
		config.tags = map[string]interface{}{"enlightened": false}
		ctx.AdminSession.requirePatchEntity(config, "tags")

		config.name = newName                              // name should not be updated
		config.data = map[string]interface{}{"foo": "bar"} // data should not have gotten updated
		ctx.AdminSession.validateUpdate(config)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.name = uuid.New().String()
		config.data = map[string]interface{}{"bim": "bom"}
		config.tags = map[string]interface{}{"enlightened": true}
		ctx.AdminSession.requirePatchEntity(config, "name", "data", "tags")
		ctx.AdminSession.validateUpdate(config)
	})

	t.Run("update of type config should fail", func(t *testing.T) {
		configType := ctx.AdminSession.requireCreateNewConfigType()
		config := ctx.AdminSession.requireCreateNewConfig(configType.id, map[string]interface{}{"port": float64(22)})
		ctx.AdminSession.validateEntityWithQuery(config)

		configType2 := ctx.AdminSession.requireCreateNewConfigType()
		config.sendType = true
		config.configType = configType2.id
		ctx.AdminSession.requireUpdateEntity(config)

		config.configType = configType.id
		ctx.AdminSession.validateEntityWithQuery(config)

		config.configType = configType2.id
		ctx.AdminSession.requirePatchEntity(config, "name", "type")

		config.configType = configType.id
		ctx.AdminSession.validateEntityWithQuery(config)
	})

	t.Run("delete should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.AdminSession.requireCreateNewConfigType()
		now := time.Now()
		config := ctx.AdminSession.requireCreateNewConfig(configType.id, map[string]interface{}{"port": float64(22)})
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		ctx.AdminSession.requireDeleteEntity(config)
		ctx.requireNotFoundError(ctx.AdminSession.query("configs/" + config.id))
	})

	ctx.enabledJsonLogging = true
	t.Run("create with schema should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType := ctx.newConfigType()
		configType.schema = map[string]interface{}{
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
		configType.id = ctx.AdminSession.requireCreateEntity(configType)

		config := ctx.newConfig(configType.id, map[string]interface{}{"port": 22})
		httpCode, body := ctx.AdminSession.createEntity(config)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "(root)")

		now := time.Now()
		config = ctx.newConfig(configType.id, map[string]interface{}{
			"hostname": "ssh.globotech.bizniz",
			"port":     float64(22),
		})
		config.id = ctx.AdminSession.requireCreateEntity(config)
		entityJson := ctx.AdminSession.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})
}
