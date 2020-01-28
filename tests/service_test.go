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
	nonAdminuserSession := ctx.AdminSession.createUserAndLogin(false, s(identityRole), nil)

	t.Run("create without name should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		service := ctx.newTestService(nil, nil)
		service.name = ""
		httpCode, body := ctx.AdminSession.createEntity(service)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "name")
	})

	t.Run("create should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		now := time.Now()
		service := ctx.AdminSession.requireNewService(nil, nil)
		service.permissions = []string{"Dial", "Bind"}
		entityJson := ctx.AdminSession.validateEntityWithQuery(service)
		ctx.validateDateFieldsForCreate(now, entityJson)
	})

	t.Run("list as admin should return 3 services", func(t *testing.T) {
		ctx.testContextChanged(t)
		service1 := ctx.AdminSession.requireNewService(nil, nil)
		service1.permissions = []string{"Dial", "Bind"}
		service2 := ctx.AdminSession.requireNewService(nil, nil)
		service2.permissions = []string{"Dial", "Bind"}
		service3 := ctx.AdminSession.requireNewService(nil, nil)
		service3.permissions = []string{"Dial", "Bind"}

		ctx.AdminSession.validateEntityWithLookup(service1)
		ctx.AdminSession.validateEntityWithQuery(service1)
		ctx.AdminSession.validateEntityWithQuery(service2)
		ctx.AdminSession.validateEntityWithQuery(service3)
	})

	t.Run("list as non-admin should return 3 services", func(t *testing.T) {
		ctx.testContextChanged(t)
		dialRole := uuid.New().String()
		bindRole := uuid.New().String()
		service1 := ctx.AdminSession.requireNewService(s(dialRole), nil)
		service1.permissions = []string{"Dial"}
		service2 := ctx.AdminSession.requireNewService(s(bindRole), nil)
		service2.permissions = []string{"Bind"}
		service3 := ctx.AdminSession.requireNewService(s(dialRole, bindRole), nil)
		service3.permissions = []string{"Dial", "Bind"}
		service4 := ctx.AdminSession.requireNewService(nil, nil)
		service5 := ctx.AdminSession.requireNewService(nil, nil)
		service6 := ctx.AdminSession.requireNewService(nil, nil)
		service7 := ctx.AdminSession.requireNewService(nil, nil)

		ctx.AdminSession.requireNewServicePolicy("Dial", s("#"+dialRole), s("#"+identityRole))
		ctx.AdminSession.requireNewServicePolicy("Bind", s("#"+bindRole), s("#"+identityRole))

		fmt.Printf("Expecting\n%v\n%v\n%v and not\n%v to be in final list\n", service1.id, service2.id, service3.id, service4.id)
		query := url.QueryEscape(fmt.Sprintf(`id in ["%v", "%v", "%v", "%v", "%v", "%v", "%v"]`,
			service1.id, service2.id, service3.id, service4.id, service5.id, service6.id, service7.id))
		result := nonAdminuserSession.requireQuery("services?filter=" + query)
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
		ctx.testContextChanged(t)
		service := ctx.AdminSession.requireNewService(nil, nil)
		service.permissions = []string{"Dial", "Bind"}
		ctx.AdminSession.validateEntityWithLookup(service)
	})

	t.Run("lookup non-existent service as admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.requireNotFoundError(ctx.AdminSession.query("services/" + uuid.New().String()))
	})

	t.Run("lookup existing service as non-admin should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		dialRole := uuid.New().String()
		bindRole := uuid.New().String()
		service1 := ctx.AdminSession.requireNewService(s(dialRole), nil)
		service1.permissions = []string{"Dial"}
		service2 := ctx.AdminSession.requireNewService(s(bindRole), nil)
		service2.permissions = []string{"Bind"}
		service3 := ctx.AdminSession.requireNewService(s(dialRole, bindRole), nil)
		service3.permissions = []string{"Dial", "Bind"}

		ctx.AdminSession.requireNewServicePolicy("Dial", s("#"+dialRole), s("#"+identityRole))
		ctx.AdminSession.requireNewServicePolicy("Bind", s("#"+bindRole), s("#"+identityRole))

		nonAdminuserSession.validateEntityWithLookup(service1)
		nonAdminuserSession.validateEntityWithLookup(service2)
		nonAdminuserSession.validateEntityWithLookup(service3)
	})

	t.Run("lookup non-existent service as non-admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.requireNotFoundError(nonAdminuserSession.query("services/" + uuid.New().String()))
	})

	t.Run("lookup non-visible service as non-admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		service := ctx.AdminSession.requireNewService(nil, nil)
		ctx.requireNotFoundError(nonAdminuserSession.query("services/" + service.id))
	})

	t.Run("update service should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		now := time.Now()
		service := ctx.AdminSession.requireNewService(nil, nil)
		service.permissions = []string{"Bind", "Dial"}
		entityJson := ctx.AdminSession.validateEntityWithQuery(service)
		createdAt := ctx.validateDateFieldsForCreate(now, entityJson)

		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		service.endpointAddress = uuid.New().String()
		ctx.AdminSession.requireUpdateEntity(service)

		result := ctx.AdminSession.requireQuery("services/" + service.id)
		jsonService := ctx.requirePath(result, "data")
		service.validate(ctx, jsonService)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonService)
	})

	t.Run("role attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		service := ctx.newTestService(nil, nil)
		service.permissions = []string{"Bind", "Dial"}
		service.edgeRouterRoles = []string{"#" + role1, "#" + role2}
		service.id = ctx.AdminSession.requireCreateEntity(service)
		ctx.AdminSession.validateEntityWithQuery(service)
		ctx.AdminSession.validateEntityWithLookup(service)
	})

	t.Run("role attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		service := ctx.newTestService(nil, nil)
		service.permissions = []string{"Bind", "Dial"}
		service.edgeRouterRoles = []string{"#" + role1, "#" + role2}
		service.id = ctx.AdminSession.requireCreateEntity(service)

		role3 := uuid.New().String()
		service.edgeRouterRoles = []string{"#" + role2, "#" + role3}
		ctx.AdminSession.requireUpdateEntity(service)
		ctx.AdminSession.validateEntityWithLookup(service)
	})
}

func Test_ServiceListWithConfigs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	configType1 := ctx.AdminSession.requireCreateNewConfigType()
	configType2 := ctx.AdminSession.requireCreateNewConfigType()

	config1 := ctx.AdminSession.requireCreateNewConfig(configType1.id, map[string]interface{}{
		"hostname": "foo",
		"port":     float64(22),
	})

	config2 := ctx.AdminSession.requireCreateNewConfig(configType2.id, map[string]interface{}{
		"dialAddress": "tcp:localhost:5432",
	})

	config3 := ctx.AdminSession.requireCreateNewConfig(configType1.name, map[string]interface{}{
		"hostname": "bar",
		"port":     float64(80),
	})

	service1 := ctx.AdminSession.requireNewService(nil, nil)
	service2 := ctx.AdminSession.requireNewService(nil, s(config1.id))
	service3 := ctx.AdminSession.requireNewService(nil, s(config2.name))
	service4 := ctx.AdminSession.requireNewService(nil, s(config2.id, config3.name))

	service1V := &configValidatingService{service: service1}
	service2V := &configValidatingService{service: service2}
	service3V := &configValidatingService{service: service3}
	service4V := &configValidatingService{service: service4}

	services := []*configValidatingService{service1V, service2V, service3V, service4V}

	ctx.AdminSession.requireNewServicePolicy("Dial", s("#all"), s("#all"))

	session := ctx.AdminSession.createUserAndLogin(false, nil, nil)
	for _, service := range services {
		service.configs = map[string]*config{}
		session.validateEntityWithQuery(service)
	}

	session = ctx.AdminSession.createUserAndLogin(false, nil, s(configType1.id))
	service2V.configs[configType1.name] = config1
	service4V.configs[configType1.name] = config3
	for _, service := range services {
		session.validateEntityWithQuery(service)
		service.configs = map[string]*config{}
	}

	session = ctx.AdminSession.createUserAndLogin(false, nil, s(configType2.id))
	service3V.configs[configType2.name] = config2
	service4V.configs[configType2.name] = config2
	for _, service := range services {
		session.validateEntityWithQuery(service)
	}

	session = ctx.AdminSession.createUserAndLogin(false, nil, s(configType1.name, configType2.id))
	service2V.configs[configType1.name] = config1
	service3V.configs[configType2.name] = config2
	service4V.configs[configType1.name] = config3
	service4V.configs[configType2.name] = config2
	for _, service := range services {
		session.validateEntityWithQuery(service)
		service.configs = map[string]*config{}
	}

	session = ctx.AdminSession.createUserAndLogin(false, nil, s("all"))
	service2V.configs[configType1.name] = config1
	service3V.configs[configType2.name] = config2
	service4V.configs[configType1.name] = config3
	service4V.configs[configType2.name] = config2
	for _, service := range services {
		session.validateEntityWithQuery(service)
		service.configs = map[string]*config{}
	}
}

func Test_ServiceListWithConfigDuplicate(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	configType1 := ctx.AdminSession.requireCreateNewConfigType()

	config1 := ctx.AdminSession.requireCreateNewConfig(configType1.id, map[string]interface{}{
		"hostname": "foo",
		"port":     float64(22),
	})

	config2 := ctx.AdminSession.requireCreateNewConfig(configType1.name, map[string]interface{}{
		"hostname": "bar",
		"port":     float64(80),
	})

	service := ctx.newTestService(nil, s(config1.name, config2.name))
	httpCode, body := ctx.AdminSession.createEntity(service)
	ctx.requireFieldError(httpCode, body, apierror.InvalidFieldCode, "configs")
}
