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
	"github.com/netfoundry/ziti-edge/controller/apierror"
	"testing"

	"github.com/google/uuid"
)

func Test_Appwan(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	user1 := ctx.createUserAndLogin(false)
	user2 := ctx.createUserAndLogin(false)
	user3 := ctx.createUserAndLogin(false)

	t.Run("create without name should fail", func(t *testing.T) {
		appwan := newTestAppwan()
		appwan.name = ""
		httpCode, body := ctx.createEntity(appwan)
		ctx.requireFieldError(httpCode, body, apierror.CouldNotValidateCode, "name")
	})

	t.Run("create with invalid identities should fail", func(t *testing.T) {
		appwan := newTestAppwan()
		invalidId := uuid.New().String()
		appwan.identities = []string{invalidId}
		httpCode, body := ctx.createEntity(appwan)
		ctx.requireMultiFieldError(httpCode, body, apierror.InvalidFieldCode, "identities", invalidId)
	})

	t.Run("create with invalid services should fail", func(t *testing.T) {
		appwan := newTestAppwan()
		invalidId := uuid.New().String()
		appwan.services = []string{invalidId}
		httpCode, body := ctx.createEntity(appwan)
		ctx.requireMultiFieldError(httpCode, body, apierror.InvalidFieldCode, "services", invalidId)
	})

	t.Run("create with no identities/services should pass", func(t *testing.T) {
		appwan := newTestAppwan()
		service := ctx.requireCreateNewService()
		ctx.requireCreateEntity(appwan)
		ctx.req.False(ctx.isServiceVisibleToUser(user1, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service.id))

	})

	t.Run("create with identities/services should pass", func(t *testing.T) {
		appwan := newTestAppwan()
		service := ctx.requireCreateNewService()
		appwan.identities = []string{user1.identityId}
		appwan.services = []string{service.id}
		appwan.id = ctx.requireCreateEntity(appwan)
		ctx.req.True(ctx.isServiceVisibleToUser(user1, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service.id))
		ctx.validateEntityWithQuery(appwan)
	})

	t.Run("add non-existent identity should fail", func(t *testing.T) {
		appwan := newTestAppwan()
		appwanId := ctx.requireCreateEntity(appwan)
		invalidId := uuid.New().String()
		httpStatus, body := ctx.addAssociation("app-wans/"+appwanId+"/identities", invalidId)
		ctx.requireMultiFieldError(httpStatus, body, apierror.InvalidFieldCode, "identities", invalidId)
	})

	t.Run("add multiple non-existent identities should fail", func(t *testing.T) {
		appwan := newTestAppwan()
		appwanId := ctx.requireCreateEntity(appwan)
		list := []string{uuid.New().String(), uuid.New().String(), uuid.New().String()}
		httpStatus, body := ctx.addAssociation("app-wans/"+appwanId+"/identities", list...)
		ctx.requireMultiFieldError(httpStatus, body, apierror.InvalidFieldCode, "identities", list...)
	})

	t.Run("add non-existent service should fail", func(t *testing.T) {
		appwan := newTestAppwan()
		appwanId := ctx.requireCreateEntity(appwan)
		invalidId := uuid.New().String()
		httpStatus, body := ctx.addAssociation("app-wans/"+appwanId+"/services", invalidId)
		ctx.requireMultiFieldError(httpStatus, body, apierror.InvalidFieldCode, "services", invalidId)
	})

	t.Run("add multiple non-existent services should fail", func(t *testing.T) {
		appwan := newTestAppwan()
		appwanId := ctx.requireCreateEntity(appwan)
		list := []string{uuid.New().String(), uuid.New().String(), uuid.New().String()}
		httpStatus, body := ctx.addAssociation("app-wans/"+appwanId+"/services", list...)
		ctx.requireMultiFieldError(httpStatus, body, apierror.InvalidFieldCode, "services", list...)
	})

	t.Run("remove non-existent service should fail", func(t *testing.T) {
		appwan := newTestAppwan()
		appwanId := ctx.requireCreateEntity(appwan)
		invalidId := uuid.New().String()
		httpStatus, body := ctx.removeAssociation("app-wans/"+appwanId+"/services", invalidId)
		ctx.requireMultiFieldError(httpStatus, body, apierror.InvalidFieldCode, "services", invalidId)
	})

	t.Run("remove multiple non-existent services should fail", func(t *testing.T) {
		appwan := newTestAppwan()
		appwanId := ctx.requireCreateEntity(appwan)
		list := []string{uuid.New().String(), uuid.New().String(), uuid.New().String()}
		httpStatus, body := ctx.removeAssociation("app-wans/"+appwanId+"/services", list...)
		ctx.requireMultiFieldError(httpStatus, body, apierror.InvalidFieldCode, "services", list...)
	})

	t.Run("create with added identities/services should pass", func(t *testing.T) {
		appwan := newTestAppwan()
		appwanId := ctx.requireCreateEntity(appwan)

		service := ctx.requireCreateNewService()
		ctx.requireAddAssociation("app-wans/"+appwanId+"/identities", user1.identityId)
		ctx.requireAddAssociation("app-wans/"+appwanId+"/services", service.id)

		ctx.req.True(ctx.isServiceVisibleToUser(user1, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service.id))
	})

	t.Run("create with service and added identities should pass", func(t *testing.T) {
		appwan := newTestAppwan()

		service := ctx.requireCreateNewService()
		appwan.services = []string{service.id}
		appwanId := ctx.requireCreateEntity(appwan)

		ctx.req.False(ctx.isServiceVisibleToUser(user1, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service.id))

		ctx.requireAddAssociation("app-wans/"+appwanId+"/identities", user1.identityId, user2.identityId)

		ctx.req.True(ctx.isServiceVisibleToUser(user1, service.id))
		ctx.req.True(ctx.isServiceVisibleToUser(user2, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service.id))
	})

	t.Run("create with identity and added services should pass", func(t *testing.T) {
		appwan := newTestAppwan()

		service1 := ctx.requireCreateNewService()
		service2 := ctx.requireCreateNewService()
		appwan.identities = []string{user1.identityId}
		appwanId := ctx.requireCreateEntity(appwan)

		ctx.req.False(ctx.isServiceVisibleToUser(user1, service1.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service1.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service1.id))

		ctx.req.False(ctx.isServiceVisibleToUser(user1, service2.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service2.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service2.id))

		ctx.requireAddAssociation("app-wans/"+appwanId+"/services", service1.id, service2.id)

		ctx.req.True(ctx.isServiceVisibleToUser(user1, service1.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service1.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service1.id))

		ctx.req.True(ctx.isServiceVisibleToUser(user1, service2.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service2.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service2.id))
	})

	t.Run("create with identities/services and added identities/services should pass", func(t *testing.T) {
		appwan := newTestAppwan()

		service1 := ctx.requireCreateNewService()
		service2 := ctx.requireCreateNewService()
		appwan.identities = []string{user1.identityId}
		appwan.services = []string{service1.id}
		appwanId := ctx.requireCreateEntity(appwan)

		ctx.req.True(ctx.isServiceVisibleToUser(user1, service1.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service1.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service1.id))

		ctx.req.False(ctx.isServiceVisibleToUser(user1, service2.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service2.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service2.id))

		ctx.requireAddAssociation("app-wans/"+appwanId+"/identities", user2.identityId)
		ctx.requireAddAssociation("app-wans/"+appwanId+"/services", service2.id)

		ctx.req.True(ctx.isServiceVisibleToUser(user1, service1.id))
		ctx.req.True(ctx.isServiceVisibleToUser(user2, service1.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service1.id))

		ctx.req.True(ctx.isServiceVisibleToUser(user1, service2.id))
		ctx.req.True(ctx.isServiceVisibleToUser(user2, service2.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service2.id))
	})

	t.Run("delete of non-existent appwan", func(t *testing.T) {
		invalidId := uuid.New().String()
		httpStatus, body := ctx.deleteEntityOfType("app-wans", invalidId)
		ctx.requireNotFoundError(httpStatus, body)
	})

	t.Run("delete should pass", func(t *testing.T) {
		appwan := newTestAppwan()

		service := ctx.requireCreateNewService()
		appwan.identities = []string{user1.identityId}
		appwan.services = []string{service.id}
		appwan.id = ctx.requireCreateEntity(appwan)

		ctx.req.True(ctx.isServiceVisibleToUser(user1, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service.id))

		ctx.requireDeleteEntity(appwan)

		ctx.req.False(ctx.isServiceVisibleToUser(user1, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user2, service.id))
		ctx.req.False(ctx.isServiceVisibleToUser(user3, service.id))
	})
}
