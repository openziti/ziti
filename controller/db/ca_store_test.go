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

package db

import (
	"testing"

	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
)

// Test_CaStore_RejectsPrefixedIdentityRoles covers the store-level backstop. Identity roles carrying
// the '#' or '@' policy reference prefixes are copied onto enrolling identities, where the identity
// store rejects them, so the CA store refuses them at the source.
func Test_CaStore_RejectsPrefixedIdentityRoles(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	t.Run("create with a role prefixed identity role fails", func(t *testing.T) {
		ctx.NextTest(t)
		err := boltztest.Create(ctx, newCaWithIdentityRoles("#badrole"))
		ctx.Error(err)

		fieldErr, ok := err.(*errorz.FieldError)
		ctx.True(ok, "expected a field error, got %T", err)
		ctx.Equal(FieldIdentityRoles, fieldErr.FieldName)
		ctx.Equal("#badrole", fieldErr.FieldValue)
	})

	t.Run("create with an entity prefixed identity role fails", func(t *testing.T) {
		ctx.NextTest(t)
		err := boltztest.Create(ctx, newCaWithIdentityRoles("@badrole"))
		ctx.Error(err)

		fieldErr, ok := err.(*errorz.FieldError)
		ctx.True(ok, "expected a field error, got %T", err)
		ctx.Equal(FieldIdentityRoles, fieldErr.FieldName)
		ctx.Equal("@badrole", fieldErr.FieldValue)
	})

	t.Run("create with acceptable identity roles succeeds", func(t *testing.T) {
		ctx.NextTest(t)
		ctx.NoError(boltztest.Create(ctx, newCaWithIdentityRoles("good")))
	})
}

func newCaWithIdentityRoles(identityRoles ...string) *Ca {
	return &Ca{
		BaseExtEntity:      *boltz.NewExtEntity(eid.New(), nil),
		Name:               eid.New(),
		Fingerprint:        eid.New(),
		CertPem:            "-",
		IdentityRoles:      identityRoles,
		IdentityNameFormat: "[caName]-[commonName]",
	}
}
