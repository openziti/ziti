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
	nonAdminuserSession := ctx.AdminSession.createUserAndLogin(false, identityRole)

	ctx.enabledJsonLogging = true
	t.Run("create without name should fail", func(t *testing.T) {
		service := ctx.newTestService()
		service.name = ""
		httpCode, body := ctx.AdminSession.createEntity(service)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "name")
	})

	t.Run("create should pass", func(t *testing.T) {
		now := time.Now()
		service := ctx.AdminSession.requireCreateNewService()
		service.permissions = []string{"Dial", "Bind"}
		entityJson := ctx.AdminSession.validateEntityWithQuery(service)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("list as admin should return 3 services", func(t *testing.T) {
		service1 := ctx.AdminSession.requireCreateNewService()
		service1.permissions = []string{"Dial", "Bind"}
		service2 := ctx.AdminSession.requireCreateNewService()
		service2.permissions = []string{"Dial", "Bind"}
		service3 := ctx.AdminSession.requireCreateNewService()
		service3.permissions = []string{"Dial", "Bind"}

		ctx.AdminSession.validateEntityWithLookup(service1)
		ctx.AdminSession.validateEntityWithQuery(service1)
		ctx.AdminSession.validateEntityWithQuery(service2)
		ctx.AdminSession.validateEntityWithQuery(service3)
	})

	t.Run("list as non-admin should return 3 services", func(t *testing.T) {
		dialRole := uuid.New().String()
		bindRole := uuid.New().String()
		service1 := ctx.AdminSession.requireCreateNewService(dialRole)
		service1.permissions = []string{"Dial"}
		service2 := ctx.AdminSession.requireCreateNewService(bindRole)
		service2.permissions = []string{"Bind"}
		service3 := ctx.AdminSession.requireCreateNewService(dialRole, bindRole)
		service3.permissions = []string{"Dial", "Bind"}
		service4 := ctx.AdminSession.requireCreateNewService()
		service5 := ctx.AdminSession.requireCreateNewService()
		service6 := ctx.AdminSession.requireCreateNewService()
		service7 := ctx.AdminSession.requireCreateNewService()

		ctx.AdminSession.requireNewServicePolicy("Dial", s("@"+dialRole), s("@"+identityRole))
		ctx.AdminSession.requireNewServicePolicy("Bind", s("@"+bindRole), s("@"+identityRole))

		fmt.Printf("Expecting\n%v\n%v\n%v and not\n%v to be in final list\n", service1.id, service2.id, service3.id, service4.id)
		query := url.QueryEscape(fmt.Sprintf(`id in ["%v", "%v", "%v", "%v", "%v", "%v", "%v"]`,
			service1.id, service2.id, service3.id, service4.id, service5.id, service6.id, service7.id))
		result := nonAdminuserSession.requireQuery("services?filter="+query)
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
		service := ctx.AdminSession.requireCreateNewService()
		service.permissions = []string{"Dial", "Bind"}
		ctx.AdminSession.validateEntityWithLookup(service)
	})

	t.Run("lookup non-existent service as admin should fail", func(t *testing.T) {
		ctx.requireNotFoundError(ctx.AdminSession.query("services/"+uuid.New().String()))
	})

	t.Run("lookup existing service as non-admin should pass", func(t *testing.T) {
		dialRole := uuid.New().String()
		bindRole := uuid.New().String()
		service1 := ctx.AdminSession.requireCreateNewService(dialRole)
		service1.permissions = []string{"Dial"}
		service2 := ctx.AdminSession.requireCreateNewService(bindRole)
		service2.permissions = []string{"Bind"}
		service3 := ctx.AdminSession.requireCreateNewService(dialRole, bindRole)
		service3.permissions = []string{"Dial", "Bind"}

		ctx.AdminSession.requireNewServicePolicy("Dial", s("@"+dialRole), s("@"+identityRole))
		ctx.AdminSession.requireNewServicePolicy("Bind", s("@"+bindRole), s("@"+identityRole))

		nonAdminuserSession.validateEntityWithLookup(service1)
		nonAdminuserSession.validateEntityWithLookup(service2)
		nonAdminuserSession.validateEntityWithLookup(service3)
	})

	t.Run("lookup non-existent service as non-admin should fail", func(t *testing.T) {
		ctx.requireNotFoundError(nonAdminuserSession.query("services/"+uuid.New().String()))
	})

	t.Run("lookup non-visible service as non-admin should fail", func(t *testing.T) {
		service := ctx.AdminSession.requireCreateNewService()
		ctx.requireNotFoundError(nonAdminuserSession.query("services/"+service.id))
	})

	t.Run("update service should pass", func(t *testing.T) {
		now := time.Now()
		service := ctx.AdminSession.requireCreateNewService()
		service.permissions = []string{"Bind", "Dial"}
		entityJson := ctx.AdminSession.validateEntityWithQuery(service)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		service.endpointAddress = uuid.New().String()
		ctx.AdminSession.requireUpdateEntity(service)

		result := ctx.AdminSession.requireQuery("services/"+service.id)
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
		service.id = ctx.AdminSession.requireCreateEntity(service)
		ctx.AdminSession.validateEntityWithQuery(service)
		ctx.AdminSession.validateEntityWithLookup(service)
	})

	ctx.enabledJsonLogging = true
	t.Run("role attributes should be updated", func(t *testing.T) {
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		service := ctx.newTestService()
		service.permissions = []string{"Bind", "Dial"}
		service.edgeRouterRoles = []string{role1, role2}
		service.id = ctx.AdminSession.requireCreateEntity(service)

		role3 := uuid.New().String()
		service.edgeRouterRoles = []string{role2, role3}
		ctx.AdminSession.requireUpdateEntity(service)
		ctx.AdminSession.validateEntityWithLookup(service)
	})
}
