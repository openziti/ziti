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
	"github.com/netfoundry/ziti-edge/edge/controller/apierror"
	"fmt"
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

	t.Run("create without name should fail", func(t *testing.T) {
		service := ctx.newTestService()
		service.name = ""
		httpCode, body := ctx.createEntity(service)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "name")
	})

	t.Run("create missing cluster should fail", func(t *testing.T) {
		service := ctx.newTestService()
		service.clusterIds = []string{}
		httpCode, body := ctx.createEntity(service)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "clusters")
	})

	t.Run("create with invalid cluster should fail", func(t *testing.T) {
		service := ctx.newTestService()
		service.clusterIds = []string{uuid.New().String()}
		httpCode, body := ctx.createEntity(service)
		parsed := ctx.requireFieldError(httpCode, body, apierror.InvalidFieldCode, "clusters")
		ctx.pathEquals(parsed, toIntfSlice(service.clusterIds), path("error.cause.value"))
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

	t.Run("add cluster should pass", func(t *testing.T) {
		service := ctx.requireCreateNewService()

		clusterId2 := ctx.requireCreateCluster(uuid.New().String())
		ctx.requireAddAssociation("services/"+service.id+"/clusters", clusterId2)

		result := ctx.requireQuery(ctx.adminSessionId, "services/"+service.id)
		jsonService := ctx.requirePath(result, "data")
		service.clusterIds = append(service.clusterIds, clusterId2)
		service.validate(ctx, jsonService)
		ctx.pathEquals(jsonService, false, path("hostable"))
	})

	t.Run("add non-existent cluster should fail", func(t *testing.T) {
		service := ctx.requireCreateNewService()
		fakeClusterId := uuid.New().String()
		httpStatus, body := ctx.addAssociation("services/"+service.id+"/clusters", fakeClusterId)
		ctx.requireMultiFieldError(httpStatus, body, apierror.InvalidFieldCode, "clusters", fakeClusterId)
	})

	t.Run("add multiple non-existent clusters should fail", func(t *testing.T) {
		service := ctx.requireCreateNewService()
		list := []string{uuid.New().String(), uuid.New().String(), uuid.New().String()}
		httpStatus, body := ctx.addAssociation("services/"+service.id+"/clusters", list...)
		ctx.requireMultiFieldError(httpStatus, body, apierror.InvalidFieldCode, "clusters", list...)
	})

	t.Run("remove cluster should pass", func(t *testing.T) {
		service := ctx.requireCreateNewService()

		clusterId2 := ctx.requireCreateCluster(uuid.New().String())
		ctx.requireAddAssociation("services/"+service.id+"/clusters", clusterId2)

		ctx.requireRemoveAssociation("services/"+service.id+"/clusters", clusterId2)

		result := ctx.requireQuery(ctx.adminSessionId, "services/"+service.id)
		jsonService := ctx.requirePath(result, "data")
		service.clusterIds = []string{ctx.clusterId}
		service.validate(ctx, jsonService)
		ctx.pathEquals(jsonService, false, path("hostable"))
	})

	t.Run("remove non-existent cluster should fail", func(t *testing.T) {
		service := ctx.requireCreateNewService()

		fakeClusterId := uuid.New().String()
		httpStatus, body := ctx.removeAssociation("services/"+service.id+"/clusters", fakeClusterId)
		ctx.requireMultiFieldError(httpStatus, body, apierror.InvalidFieldCode, "clusters", fakeClusterId)

		result := ctx.requireQuery(ctx.adminSessionId, "services/"+service.id)
		jsonService := ctx.requirePath(result, "data")
		service.validate(ctx, jsonService)
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
}
