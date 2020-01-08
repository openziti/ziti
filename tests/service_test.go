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
	"net/url"
	"testing"
	"time"

	"github.com/netfoundry/ziti-edge/controller/apierror"

	"github.com/google/uuid"
)

func Test_Services(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	identityRole := uuid.New().String()
	nonAdminUser := ctx.createUserAndLogin(false, identityRole)

	ctx.enabledJsonLogging = true
	t.Run("create without name should fail", func(t *testing.T) {
		service := ctx.newTestService()
		service.name = ""
		httpCode, body := ctx.createEntity(service)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "name")
	})

	t.Run("create should pass", func(t *testing.T) {
		now := time.Now()
		service := ctx.requireCreateNewService()
		service.permissions = []string{"Dial", "Bind"}
		entityJson := ctx.validateEntityWithQuery(service)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("list as admin should return 3 services", func(t *testing.T) {
		service1 := ctx.requireCreateNewService()
		service1.permissions = []string{"Dial", "Bind"}
		service2 := ctx.requireCreateNewService()
		service2.permissions = []string{"Dial", "Bind"}
		service3 := ctx.requireCreateNewService()
		service3.permissions = []string{"Dial", "Bind"}

		ctx.validateEntityWithLookup(service1)
		ctx.validateEntityWithQuery(service1)
		ctx.validateEntityWithQuery(service2)
		ctx.validateEntityWithQuery(service3)
	})

	t.Run("list as non-admin should return 3 services", func(t *testing.T) {
		dialRole := uuid.New().String()
		bindRole := uuid.New().String()
		service1 := ctx.requireCreateNewService(dialRole)
		service1.permissions = []string{"Dial"}
		service2 := ctx.requireCreateNewService(bindRole)
		service2.permissions = []string{"Bind"}
		service3 := ctx.requireCreateNewService(dialRole, bindRole)
		service3.permissions = []string{"Dial", "Bind"}
		service4 := ctx.requireCreateNewService()
		service5 := ctx.requireCreateNewService()
		service6 := ctx.requireCreateNewService()
		service7 := ctx.requireCreateNewService()

		ctx.requireNewServicePolicy("Dial", s("@"+dialRole), s("@"+identityRole))
		ctx.requireNewServicePolicy("Bind", s("@"+bindRole), s("@"+identityRole))

		fmt.Printf("Expecting\n%v\n%v\n%v and not\n%v to be in final list\n", service1.id, service2.id, service3.id, service4.id)
		query := url.QueryEscape(fmt.Sprintf(`id in ["%v", "%v", "%v", "%v", "%v", "%v", "%v"]`,
			service1.id, service2.id, service3.id, service4.id, service5.id, service6.id, service7.id))
		result := ctx.requireQuery(nonAdminUser.sessionId, "services?filter="+query)
		data := ctx.requirePath(result, "data")
		ctx.requireNoChildWith(data, "id", service4.id)
		ctx.requireNoChildWith(data, "id", service5.id)
		ctx.requireNoChildWith(data, "id", service6.id)
		ctx.requireNoChildWith(data, "id", service7.id)

		jsonService := ctx.requireChildWith(data, "id", service1.id)
		service1.validate(ctx, jsonService)
		jsonService = ctx.requireChildWith(data, "id", service2.id)
		service2.validate(ctx, jsonService)
		jsonService = ctx.requireChildWith(data, "id", service3.id)
		service3.validate(ctx, jsonService)
	})

	t.Run("lookup as admin should pass", func(t *testing.T) {
		service := ctx.requireCreateNewService()
		service.permissions = []string{"Dial", "Bind"}
		ctx.validateEntityWithLookup(service)
	})

	t.Run("lookup non-existent service as admin should fail", func(t *testing.T) {
		ctx.requireNotFoundError(ctx.query(ctx.adminSessionId, "services/"+uuid.New().String()))
	})

	t.Run("lookup existing service as non-admin should pass", func(t *testing.T) {
		dialRole := uuid.New().String()
		bindRole := uuid.New().String()
		service1 := ctx.requireCreateNewService(dialRole)
		service1.permissions = []string{"Dial"}
		service2 := ctx.requireCreateNewService(bindRole)
		service2.permissions = []string{"Bind"}
		service3 := ctx.requireCreateNewService(dialRole, bindRole)
		service3.permissions = []string{"Dial", "Bind"}

		ctx.requireNewServicePolicy("Dial", s("@"+dialRole), s("@"+identityRole))
		ctx.requireNewServicePolicy("Bind", s("@"+bindRole), s("@"+identityRole))

		ctx.validateEntityWithLookupAndSession(nonAdminUser.sessionId, service1)
		ctx.validateEntityWithLookupAndSession(nonAdminUser.sessionId, service2)
		ctx.validateEntityWithLookupAndSession(nonAdminUser.sessionId, service3)
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
		service.permissions = []string{"Bind", "Dial"}
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
		service.permissions = []string{"Bind", "Dial"}
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
		service.permissions = []string{"Bind", "Dial"}
		service.edgeRouterRoles = []string{role1, role2}
		service.id = ctx.requireCreateEntity(service)

		role3 := uuid.New().String()
		service.edgeRouterRoles = []string{role2, role3}
		ctx.requireUpdateEntity(service)
		ctx.validateEntityWithLookup(service)
	})
}
