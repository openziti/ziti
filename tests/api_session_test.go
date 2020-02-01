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
	"github.com/google/uuid"
	"sort"
	"testing"
)

func Test_ApiSession(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()

	ctx.requireAdminLogin()

	t.Run("config types should be set and viewable", func(t *testing.T) {
		configType1 := ctx.AdminSession.requireCreateNewConfigType()
		configType2 := ctx.AdminSession.requireCreateNewConfigType()

		auth := &updbAuthenticator{
			Username:    uuid.New().String(),
			Password:    uuid.New().String(),
			ConfigTypes: s(configType1.id, configType2.name),
		}

		_ = ctx.AdminSession.requireCreateIdentityWithUpdbEnrollment(auth.Username, auth.Password, false)
		session, err := auth.Authenticate(ctx)
		ctx.req.NoError(err)

		expected := s(configType1.id, configType2.id)
		sort.Strings(expected)
		sort.Strings(session.configTypes)
		ctx.req.Equal(expected, session.configTypes)
	})
}
