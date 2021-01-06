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
	"fmt"
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/util/stringz"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/openziti/edge/controller/apierror"
)

func Test_Services(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	identityRole := eid.New()
	nonAdminUserSession := ctx.AdminSession.createUserAndLogin(false, s(identityRole), nil)

	t.Run("create without name should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		service := ctx.newService(nil, nil)
		service.Name = ""
		resp := ctx.AdminSession.createEntity(service)
		ctx.requireFieldError(resp.StatusCode(), resp.Body(), apierror.CouldNotValidateCode, "name")
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
		dialRole := eid.New()
		bindRole := eid.New()
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

		ctx.AdminSession.requireNewServicePolicy("Dial", s("#"+dialRole), s("#"+identityRole), s())
		ctx.AdminSession.requireNewServicePolicy("Bind", s("#"+bindRole), s("#"+identityRole), s())

		query := url.QueryEscape(fmt.Sprintf(`id in ["%v", "%v", "%v", "%v", "%v", "%v", "%v"]`,
			service1.Id, service2.Id, service3.Id, service4.Id, service5.Id, service6.Id, service7.Id))
		result := nonAdminUserSession.requireQuery("services?filter=" + query)
		data := ctx.RequireGetNonNilPathValue(result, "data")
		ctx.RequireNoChildWith(data, "id", service4.Id)
		ctx.RequireNoChildWith(data, "id", service5.Id)
		ctx.RequireNoChildWith(data, "id", service6.Id)
		ctx.RequireNoChildWith(data, "id", service7.Id)

		jsonService := ctx.RequireChildWith(data, "id", service1.Id)
		service1.validate(ctx, jsonService)
		jsonService = ctx.RequireChildWith(data, "id", service2.Id)
		service2.validate(ctx, jsonService)
		jsonService = ctx.RequireChildWith(data, "id", service3.Id)
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
		ctx.RequireNotFoundError(ctx.AdminSession.query("services/" + eid.New()))
	})

	t.Run("lookup existing service as non-admin should pass", func(t *testing.T) {
		ctx.testContextChanged(t)
		dialRole := eid.New()
		bindRole := eid.New()
		service1 := ctx.AdminSession.requireNewService(s(dialRole), nil)
		service1.permissions = []string{"Dial"}
		service2 := ctx.AdminSession.requireNewService(s(bindRole), nil)
		service2.permissions = []string{"Bind"}
		service3 := ctx.AdminSession.requireNewService(s(dialRole, bindRole), nil)
		service3.permissions = []string{"Dial", "Bind"}

		ctx.AdminSession.requireNewServicePolicy("Dial", s("#"+dialRole), s("#"+identityRole), s())
		ctx.AdminSession.requireNewServicePolicy("Bind", s("#"+bindRole), s("#"+identityRole), s())

		nonAdminUserSession.validateEntityWithLookup(service1)
		nonAdminUserSession.validateEntityWithLookup(service2)
		nonAdminUserSession.validateEntityWithLookup(service3)
	})

	t.Run("lookup non-existent service as non-admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		ctx.RequireNotFoundError(nonAdminUserSession.query("services/" + eid.New()))
	})

	t.Run("query non-visible service as non-admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		service := ctx.AdminSession.requireNewService(nil, nil)
		query := url.QueryEscape(fmt.Sprintf(`id in ["%v"]`, service.Id))
		body := nonAdminUserSession.requireQuery("services?filter=" + query)
		data := body.S("data")
		children, err := data.Children()
		ctx.Req.True(data == nil || data.Data() == nil || (err == nil && len(children) == 0))
	})

	t.Run("lookup non-visible service as non-admin should fail", func(t *testing.T) {
		ctx.testContextChanged(t)
		service := ctx.AdminSession.requireNewService(nil, nil)
		httpStatus, body := nonAdminUserSession.query("services/" + service.Id)
		ctx.logJson(body)
		ctx.RequireNotFoundError(httpStatus, body)
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
		service.terminatorStrategy = "ha"
		ctx.AdminSession.requireUpdateEntity(service)

		result := ctx.AdminSession.requireQuery("services/" + service.Id)
		jsonService := ctx.RequireGetNonNilPathValue(result, "data")
		service.validate(ctx, jsonService)
		ctx.validateDateFieldsForUpdate(now, createdAt, jsonService)
	})
}

func Test_ServiceListWithConfigs(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	configType1 := ctx.AdminSession.requireCreateNewConfigTypeWithPrefix("ONE")
	configType2 := ctx.AdminSession.requireCreateNewConfigTypeWithPrefix("TWO")
	configType3 := ctx.AdminSession.requireCreateNewConfigTypeWithPrefix("THREE")

	config1 := ctx.AdminSession.requireCreateNewConfig(configType1.Id, map[string]interface{}{
		"hostname": "foo",
		"port":     float64(22),
	})

	config2 := ctx.AdminSession.requireCreateNewConfig(configType2.Id, map[string]interface{}{
		"dialAddress": "tcp:localhost:5432",
	})

	config3 := ctx.AdminSession.requireCreateNewConfig(configType1.Id, map[string]interface{}{
		"hostname": "bar",
		"port":     float64(80),
	})

	config4 := ctx.AdminSession.requireCreateNewConfig(configType2.Id, map[string]interface{}{
		"dialAddress": "udp:external:5432",
	})

	config5 := ctx.AdminSession.requireCreateNewConfig(configType3.Id, map[string]interface{}{
		"froboz": "schnapplecakes",
	})

	service1 := ctx.AdminSession.requireNewService(nil, nil)
	service2 := ctx.AdminSession.requireNewService(nil, s(config1.Id))
	service3 := ctx.AdminSession.requireNewService(nil, s(config2.Id))
	service4 := ctx.AdminSession.requireNewService(nil, s(config2.Id, config3.Id))

	ctx.AdminSession.validateAssociations(service4, "configs", config2, config3)

	service1V := &configValidatingService{service: service1}
	service2V := &configValidatingService{service: service2}
	service3V := &configValidatingService{service: service3}
	service4V := &configValidatingService{service: service4}

	services := []*configValidatingService{service1V, service2V, service3V, service4V}

	ctx.AdminSession.requireNewServicePolicy("Dial", s("#all"), s("#all"), s())

	session := ctx.AdminSession.createUserAndLogin(false, nil, nil)
	for _, service := range services {
		service.configs = map[string]*Config{}
		session.validateEntityWithQuery(service)
	}

	session = ctx.AdminSession.createUserAndLogin(false, nil, s(configType1.Id))
	service2V.configs[configType1.Name] = config1
	service4V.configs[configType1.Name] = config3
	for _, service := range services {
		session.validateEntityWithQuery(service)
		service.configs = map[string]*Config{}
	}

	session = ctx.AdminSession.createUserAndLogin(false, nil, s(configType2.Id))
	service3V.configs[configType2.Name] = config2
	service4V.configs[configType2.Name] = config2
	for _, service := range services {
		session.validateEntityWithQuery(service)
	}

	session = ctx.AdminSession.createUserAndLogin(false, nil, s(configType1.Id, configType2.Id))
	service2V.configs[configType1.Name] = config1
	service3V.configs[configType2.Name] = config2
	service4V.configs[configType1.Name] = config3
	service4V.configs[configType2.Name] = config2
	for _, service := range services {
		session.validateEntityWithQuery(service)
		service.configs = map[string]*Config{}
	}

	session = ctx.AdminSession.createUserAndLogin(false, nil, s("all"))
	service2V.configs[configType1.Name] = config1
	service3V.configs[configType2.Name] = config2
	service4V.configs[configType1.Name] = config3
	service4V.configs[configType2.Name] = config2
	for _, service := range services {
		session.validateEntityWithQuery(service)
	}

	configs1 := []serviceConfig{{ServiceId: service4.Id, ConfigId: config1.Id}, {ServiceId: service4.Id, ConfigId: config5.Id}}
	ctx.AdminSession.requireAssignIdentityServiceConfigs(session.identityId, configs1...)
	configs1 = []serviceConfig{{ServiceId: service4.Id, ConfigId: config1.Id}, {ServiceId: service4.Id, ConfigId: config5.Id}}
	sort.Sort(sortableServiceConfigSlice(configs1))
	currentConfigs := ctx.AdminSession.listIdentityServiceConfigs(session.identityId)
	ctx.Req.Equal(configs1, currentConfigs)

	configs2 := []serviceConfig{{ServiceId: service1.Id, ConfigId: config5.Id}, {ServiceId: service3.Id, ConfigId: config1.Id}, {ServiceId: service3.Id, ConfigId: config4.Id}}
	ctx.AdminSession.requireAssignIdentityServiceConfigs(session.identityId, configs2...)
	checkConfigs := []serviceConfig{
		{ServiceId: service4.Id, ConfigId: config1.Id},
		{ServiceId: service4.Id, ConfigId: config5.Id},
		{ServiceId: service1.Id, ConfigId: config5.Id},
		{ServiceId: service3.Id, ConfigId: config1.Id},
		{ServiceId: service3.Id, ConfigId: config4.Id},
	}
	sort.Sort(sortableServiceConfigSlice(checkConfigs))
	currentConfigs = ctx.AdminSession.listIdentityServiceConfigs(session.identityId)
	ctx.Req.Equal(checkConfigs, currentConfigs)

	service1V.configs[configType3.Name] = config5
	service3V.configs[configType1.Name] = config1
	service3V.configs[configType2.Name] = config4
	service4V.configs[configType1.Name] = config1
	service4V.configs[configType3.Name] = config5
	for _, service := range services {
		session.validateEntityWithQuery(service)
		service.configs = map[string]*Config{}
	}

	ctx.AdminSession.requireRemoveIdentityServiceConfigs(session.identityId, serviceConfig{ServiceId: service1.Id, ConfigId: config5.Id}, serviceConfig{ServiceId: service3.Id, ConfigId: config1.Id})
	currentConfigs = ctx.AdminSession.listIdentityServiceConfigs(session.identityId)
	checkConfigs = []serviceConfig{
		{ServiceId: service4.Id, ConfigId: config1.Id},
		{ServiceId: service4.Id, ConfigId: config5.Id},
		{ServiceId: service3.Id, ConfigId: config4.Id},
	}
	sort.Sort(sortableServiceConfigSlice(checkConfigs))
	ctx.Req.Equal(checkConfigs, currentConfigs)

	service2V.configs[configType1.Name] = config1
	service3V.configs[configType2.Name] = config4
	service4V.configs[configType1.Name] = config1
	service4V.configs[configType2.Name] = config2
	service4V.configs[configType3.Name] = config5
	for _, service := range services {
		session.validateEntityWithQuery(service)
		service.configs = map[string]*Config{}
	}

	ctx.AdminSession.requireRemoveIdentityServiceConfigs(session.identityId)
	currentConfigs = ctx.AdminSession.listIdentityServiceConfigs(session.identityId)
	ctx.Req.Equal(0, len(currentConfigs))

	service2V.configs[configType1.Name] = config1
	service3V.configs[configType2.Name] = config2
	service4V.configs[configType1.Name] = config3
	service4V.configs[configType2.Name] = config2
	for _, service := range services {
		session.validateEntityWithQuery(service)
	}
}

func Test_ServiceListWithConfigDuplicate(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	configType1 := ctx.AdminSession.requireCreateNewConfigType()

	config1 := ctx.AdminSession.requireCreateNewConfig(configType1.Id, map[string]interface{}{
		"hostname": "foo",
		"port":     float64(22),
	})

	config2 := ctx.AdminSession.requireCreateNewConfig(configType1.Id, map[string]interface{}{
		"hostname": "bar",
		"port":     float64(80),
	})

	service := ctx.newService(nil, s(config1.Id, config2.Id))
	resp := ctx.AdminSession.createEntity(service)
	ctx.requireFieldError(resp.StatusCode(), resp.Body(), apierror.CouldNotValidateCode, "configs")
}

func Test_ServiceRoleAttributes(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	t.Run("role attributes should be created", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		service := ctx.AdminSession.requireNewService(s(role1, role2), nil)
		service.permissions = []string{"Dial", "Bind"}

		ctx.AdminSession.validateEntityWithQuery(service)
		ctx.AdminSession.validateEntityWithLookup(service)
	})

	t.Run("role attributes should be updated", func(t *testing.T) {
		ctx.testContextChanged(t)
		role1 := eid.New()
		role2 := eid.New()
		service := ctx.AdminSession.requireNewService(s(role1, role2), nil)
		service.permissions = []string{"Dial", "Bind"}

		role3 := eid.New()
		service.roleAttributes = []string{role2, role3}
		ctx.AdminSession.requireUpdateEntity(service)
		ctx.AdminSession.validateEntityWithLookup(service)
	})

	t.Run("role attributes should be queryable", func(t *testing.T) {
		ctx.testContextChanged(t)
		prefix := "rol3-attribut3-qu3ry-t3st-"
		role1 := prefix + "sales"
		role2 := prefix + "support"
		role3 := prefix + "engineering"
		role4 := prefix + "field-ops"
		role5 := prefix + "executive"

		ctx.AdminSession.requireNewService(s(role1, role2), nil)
		ctx.AdminSession.requireNewService(s(role2, role3), nil)
		ctx.AdminSession.requireNewService(s(role3, role4), nil)
		service := ctx.AdminSession.requireNewService(s(role5), nil)
		ctx.AdminSession.requireNewService(nil, nil)

		list := ctx.AdminSession.requireList("service-role-attributes")
		ctx.Req.True(len(list) >= 5)
		ctx.Req.True(stringz.ContainsAll(list, role1, role2, role3, role4, role5))

		filter := url.QueryEscape(`id contains "e" and id contains "` + prefix + `" sort by id`)
		list = ctx.AdminSession.requireList("service-role-attributes?filter=" + filter)
		ctx.Req.Equal(4, len(list))

		expected := []string{role1, role3, role4, role5}
		sort.Strings(expected)
		ctx.Req.Equal(expected, list)

		service.roleAttributes = nil
		ctx.AdminSession.requireUpdateEntity(service)
		list = ctx.AdminSession.requireList("service-role-attributes")
		ctx.Req.True(len(list) >= 4)
		ctx.Req.True(stringz.ContainsAll(list, role1, role2, role3, role4))
		ctx.Req.False(stringz.Contains(list, role5))
	})
}
