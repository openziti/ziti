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
	"testing"

	"github.com/google/uuid"
)

func Test_Identity(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	t.Run("role attributes should be created", func(t *testing.T) {
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		identity := newTestIdentity(false, role1, role2)
		identity.id = ctx.requireCreateEntity(identity)
		ctx.validateEntityWithQuery(identity)
		ctx.validateEntityWithLookup(identity)
	})

	ctx.enabledJsonLogging = true
	t.Run("role attributes should be updated", func(t *testing.T) {
		role1 := uuid.New().String()
		role2 := uuid.New().String()
		identity := newTestIdentity(false, role1, role2)
		identity.id = ctx.requireCreateEntity(identity)

		role3 := uuid.New().String()
		identity.roleAttributes = []string{role2, role3}
		ctx.requireUpdateEntity(identity)
		ctx.validateEntityWithLookup(identity)
	})
}
