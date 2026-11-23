//go:build apitests

/*
	Copyright NetFoundry Inc.

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
	"testing"

	"github.com/openziti/ziti/v2/controller/permissions"
)

// Test_Authenticator_Updb_Secrets_Not_Queryable locks in that the password hash and salt of a UPDB
// authenticator can't be used in a filter or sort. The values are never rendered, but a filter or
// sort on them answers one character at a time, which gives anyone allowed to list authenticators
// the material for an offline attack on any password, the default admin's included.
func Test_Authenticator_Updb_Secrets_Not_Queryable(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()

	adminHelper := ctx.NewEdgeManagementApi(nil)
	_, err := adminHelper.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	_, readOnlyCreds, err := adminHelper.CreateAndEnrollOttIdentityWithPermissions(false, []string{permissions.AdminReadOnlyPermission})
	ctx.Req.NoError(err)
	readOnlyCreds.CaPool = ctx.ControllerCaPool()

	readOnlyHelper := ctx.NewEdgeManagementApi(nil)
	_, err = readOnlyHelper.Authenticate(readOnlyCreds, nil)
	ctx.Req.NoError(err)

	adminUpdb := fmt.Sprintf(`method = "updb" and updbUsername = "%s"`, ctx.AdminAuthenticator.Username)

	t.Run("a read-only caller can filter on the username", func(t *testing.T) {
		ctx.testContextChanged(t)

		authenticators, err := readOnlyHelper.ListAuthenticatorsByFilter(adminUpdb)

		ctx.Req.NoError(err)
		ctx.Req.Len(authenticators, 1, "the admin's updb authenticator must be visible for the rejections below to mean anything")
	})

	for _, field := range []string{"updbPassword", "updbSalt"} {
		for _, query := range []struct {
			name   string
			filter string
		}{
			{"a range comparison", fmt.Sprintf(`%s and %s >= "a"`, adminUpdb, field)},
			{"contains", fmt.Sprintf(`%s and %s contains "a"`, adminUpdb, field)},
			{"a sort", fmt.Sprintf(`%s sort by %s`, adminUpdb, field)},
		} {
			t.Run(fmt.Sprintf("a read-only caller is refused %s on %s", query.name, field), func(t *testing.T) {
				ctx.testContextChanged(t)

				_, err := readOnlyHelper.ListAuthenticatorsByFilter(query.filter)

				ctx.Req.ApiErrorWithCode(err, "INVALID_FILTER")
			})

			t.Run(fmt.Sprintf("an admin is refused %s on %s", query.name, field), func(t *testing.T) {
				ctx.testContextChanged(t)

				_, err := adminHelper.ListAuthenticatorsByFilter(query.filter)

				ctx.Req.ApiErrorWithCode(err, "INVALID_FILTER")
			})
		}
	}
}
