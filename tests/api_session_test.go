//go:build apitests
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
	"github.com/openziti/edge/eid"
	"sort"
	"testing"
)

func Test_ApiSession(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	ctx.RequireAdminManagementApiLogin()

	t.Run("config types should be set and viewable", func(t *testing.T) {
		ctx.testContextChanged(t)
		configType1 := ctx.AdminManagementSession.requireCreateNewConfigType()
		configType2 := ctx.AdminManagementSession.requireCreateNewConfigType()

		_, auth := ctx.AdminManagementSession.requireCreateIdentityWithUpdbEnrollment(eid.New(), eid.New(), false)

		auth.ConfigTypes = s(configType1.Id, configType2.Name)

		session, err := auth.AuthenticateClientApi(ctx)
		ctx.Req.NoError(err)

		expected := s(configType1.Id, configType2.Id)
		sort.Strings(expected)
		sort.Strings(session.configTypes)
		ctx.Req.Equal(expected, session.configTypes)
	})
}
