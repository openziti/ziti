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
	nonAdminUser := ctx.createUserAndLogin(false, identityRole)

	t.Run("create without name should fail", func(t *testing.T) {
		config := ctx.newTestConfig(map[string]interface{}{"port": 22})
		config.name = ""
		httpCode, body := ctx.createEntity(config)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "name")
	})

	t.Run("create without data should fail", func(t *testing.T) {
		config := ctx.newTestConfig(nil)
		httpCode, body := ctx.createEntity(config)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "data")
	})

	t.Run("create should pass", func(t *testing.T) {
		now := time.Now()
		config := ctx.requireCreateNewConfig(map[string]interface{}{"port": float64(22)})
		entityJson := ctx.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		entityJson = ctx.validateEntityWithLookup(config)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("create with nested values should pass", func(t *testing.T) {
		now := time.Now()
		config := ctx.newTestConfig(map[string]interface{}{
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
		config.id = ctx.requireCreateEntity(config)
		entityJson := ctx.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		entityJson = ctx.validateEntityWithLookup(config)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("lookup non-existent config as admin should fail", func(t *testing.T) {
		ctx.requireNotFoundError(ctx.query(ctx.adminSessionId, "configs/"+uuid.New().String()))
	})

	t.Run("lookup config as non-admin should fail", func(t *testing.T) {
		config := ctx.requireCreateNewConfig(map[string]interface{}{"port": 22})
		ctx.requireUnauthorizedError(ctx.query(nonAdminUser.sessionId, "configs/"+config.id))
	})

	t.Run("update config should pass", func(t *testing.T) {
		now := time.Now()
		config := ctx.requireCreateNewConfig(map[string]interface{}{"port": float64(22)})
		entityJson := ctx.validateEntityWithQuery(config)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.data = map[string]interface{}{"snafu": false}
		ctx.requireUpdateEntity(config)
		jsonConfig := ctx.validateUpdate(config)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonConfig)
	})

	ctx.enabledJsonLogging = true
	t.Run("patch config should pass", func(t *testing.T) {
		now := time.Now()
		config := ctx.requireCreateNewConfig(map[string]interface{}{"port": float64(22)})
		entityJson := ctx.validateEntityWithQuery(config)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		newName := uuid.New().String()
		config.name = newName
		config.data = map[string]interface{}{"foo": "bar"}
		config.tags = map[string]interface{}{"baz": "bam"}
		ctx.requirePatchEntity(config, "name")

		config.data = map[string]interface{}{"port": float64(22)} // data should not have gotten updated
		config.tags = nil                                         // tags should not be updated
		jsonConfig := ctx.validateUpdate(config)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonConfig)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.name = uuid.New().String()
		config.data = map[string]interface{}{"foo": "bar"}
		config.tags = map[string]interface{}{"baz": "bam"}
		ctx.requirePatchEntity(config, "data")

		config.name = newName // name should not be updated
		config.tags = nil     // tags should not be updated
		ctx.validateUpdate(config)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.name = uuid.New().String()
		config.data = map[string]interface{}{"bim": "bam"}
		config.tags = map[string]interface{}{"enlightened": false}
		ctx.requirePatchEntity(config, "tags")

		config.name = newName                              // name should not be updated
		config.data = map[string]interface{}{"foo": "bar"} // data should not have gotten updated
		ctx.validateUpdate(config)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		config.name = uuid.New().String()
		config.data = map[string]interface{}{"bim": "bom"}
		config.tags = map[string]interface{}{"enlightened": true}
		ctx.requirePatchEntity(config, "name", "data", "tags")
		ctx.validateUpdate(config)
	})

	t.Run("delete should pass", func(t *testing.T) {
		now := time.Now()
		config := ctx.requireCreateNewConfig(map[string]interface{}{"port": float64(22)})
		entityJson := ctx.validateEntityWithQuery(config)
		ctx.validateDateFieldsForCreate(now, entityJson)

		ctx.requireDeleteEntity(config)
		ctx.requireNotFoundError(ctx.query(ctx.adminSessionId, "configs/"+uuid.New().String()))
	})
}
