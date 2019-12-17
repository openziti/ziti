// +build apitests

/*
	Copyright 2019 Netfoundry, Inc.

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
	"fmt"
	"github.com/netfoundry/ziti-edge/edge/controller/apierror"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

func Test_Services(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	nonAdminUser := ctx.createUserAndLogin(false)

	ctx.enabledJsonLogging = true
	t.Run("create without name should fail", func(t *testing.T) {
		service := ctx.newTestService()
		service.name = ""
		httpCode, body := ctx.createEntity(service)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "name")
	})

	t.Run("create with invalid hostId should fail", func(t *testing.T) {
		service := ctx.newTestService()
		service.hostIds = []string{uuid.New().String()}
		httpCode, body := ctx.createEntity(service)
		parsed := ctx.requireFieldError(httpCode, body, apierror.InvalidFieldCode, "hostIds")
		ctx.pathEquals(parsed, toIntfSlice(service.hostIds), path("error.cause.value"))
	})

	t.Run("create with no host ids should pass", func(t *testing.T) {
		now := time.Now()
		service := ctx.requireCreateNewService()
		entityJson := ctx.validateEntityWithQuery(service)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("create with host id should pass", func(t *testing.T) {
		service := ctx.newTestService()
		service.hostIds = []string{nonAdminUser.identityId}
		service.id = ctx.requireCreateEntity(service)
		ctx.validateEntityWithQuery(service)
	})

	t.Run("list as admin should return 3 services", func(t *testing.T) {
		service1 := ctx.requireCreateNewService()
		service2 := ctx.requireCreateNewService()
		service3 := ctx.newTestService()
		service3.hostIds = []string{nonAdminUser.identityId}
		service3.id = ctx.requireCreateEntity(service3)

		jsonService := ctx.validateEntityWithQuery(service1)
		ctx.pathEquals(jsonService, false, path("hostable"))
		jsonService = ctx.validateEntityWithQuery(service2)
		ctx.pathEquals(jsonService, false, path("hostable"))
		jsonService = ctx.validateEntityWithQuery(service3)
		ctx.pathEquals(jsonService, false, path("hostable"))
	})

	t.Run("list as non-admin should return 2 services", func(t *testing.T) {
		service1 := ctx.requireCreateNewService()
		service2 := ctx.requireCreateNewService()
		service3 := ctx.newTestService()
		service3.hostIds = []string{nonAdminUser.identityId}
		service3.id = ctx.requireCreateEntity(service3)

		appWan := &testAppwan{
			name:       uuid.New().String(),
			identities: []string{nonAdminUser.identityId},
			services:   []string{service2.id, service3.id},
		}
		ctx.requireCreateEntity(appWan)

		query := url.QueryEscape(fmt.Sprintf(`id in ["%v", "%v", "%v"]`, service1.id, service2.id, service3.id))
		result := ctx.requireQuery(nonAdminUser.sessionId, "services?filter="+query)
		data := ctx.requirePath(result, "data")
		ctx.requireNoChildWith(data, "id", service1.id)

		jsonService := ctx.requireChildWith(data, "id", service2.id)
		service2.validate(ctx, jsonService)
		ctx.pathEquals(jsonService, false, path("hostable"))

		jsonService = ctx.requireChildWith(data, "id", service3.id)
		service3.validate(ctx, jsonService)
		ctx.pathEquals(jsonService, true, path("hostable"))
	})

	t.Run("lookup as admin should pass", func(t *testing.T) {
		service := ctx.requireCreateNewService()
		jsonService := ctx.validateEntityWithLookup(service)
		ctx.pathEquals(jsonService, false, path("hostable"))
	})

	t.Run("lookup non-existent service as admin should fail", func(t *testing.T) {
		ctx.requireNotFoundError(ctx.query(ctx.adminSessionId, "services/"+uuid.New().String()))
	})

	t.Run("lookup existing service as non-admin should pass", func(t *testing.T) {
		service := ctx.requireCreateNewService()
		service2 := ctx.newTestService()
		service2.hostIds = []string{nonAdminUser.identityId}
		service2.id = ctx.requireCreateEntity(service2)

		appwan := newTestAppwan()
		appwan.identities = []string{nonAdminUser.identityId}
		appwan.services = []string{service.id, service2.id}
		ctx.requireCreateEntity(appwan)

		jsonService := ctx.validateEntityWithLookupAndSession(nonAdminUser.sessionId, service)
		ctx.pathEquals(jsonService, false, path("hostable"))

		jsonService = ctx.validateEntityWithLookupAndSession(nonAdminUser.sessionId, service2)
		ctx.pathEquals(jsonService, true, path("hostable"))
	})

	t.Run("lookup non-existent service as non-admin should fail", func(t *testing.T) {
		ctx.requireNotFoundError(ctx.query(nonAdminUser.sessionId, "services/"+uuid.New().String()))
	})

	t.Run("lookup non-visible service as non-admin should fail", func(t *testing.T) {
		service := ctx.requireCreateNewService()
		ctx.requireNotFoundError(ctx.query(nonAdminUser.sessionId, "services/"+service.id))
	})

	t.Run("update service should pass", func(t *testing.T) {
		now := time.Now()
		service := ctx.requireCreateNewService()
		entityJson := ctx.validateEntityWithQuery(service)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		service.endpointAddress = uuid.New().String()
		ctx.requireUpdateEntity(service)

		result := ctx.requireQuery(ctx.adminSessionId, "services/"+service.id)
		jsonService := ctx.requirePath(result, "data")
		service.validate(ctx, jsonService)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonService)
	})

	t.Run("role attributes should be created", func(t *testing.T) {
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		service := ctx.newTestService()
		service.edgeRouterRoles = []string{role1, role2}
		service.id = ctx.requireCreateEntity(service)
		ctx.validateEntityWithQuery(service)
		ctx.validateEntityWithLookup(service)
	})

	ctx.enabledJsonLogging = true
	t.Run("role attributes should be updated", func(t *testing.T) {
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		service := ctx.newTestService()
		service.edgeRouterRoles = []string{role1, role2}
		service.id = ctx.requireCreateEntity(service)

		role3 := uuid.New().String()
		service.edgeRouterRoles = []string{role2, role3}
		ctx.requireUpdateEntity(service)
		ctx.validateEntityWithLookup(service)
	})
}
