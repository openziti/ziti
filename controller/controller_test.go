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

package controller

import (
	"path/filepath"
	"testing"

	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

func TestValidateMigrationSourceDb(t *testing.T) {
	t.Run("rejects a db with no identities", func(t *testing.T) {
		// db.Open creates the root bucket but no identities, mimicking an empty or stray file.
		sourceDb, err := db.Open(filepath.Join(t.TempDir(), "empty.db"))
		require.NoError(t, err)
		defer func() { _ = sourceDb.Close() }()

		require.Error(t, validateMigrationSourceDb(sourceDb))
	})

	t.Run("rejects a db whose identities include no default admin", func(t *testing.T) {
		sourceDb, err := db.Open(filepath.Join(t.TempDir(), "no-admin.db"))
		require.NoError(t, err)
		defer func() { _ = sourceDb.Close() }()

		require.NoError(t, addIdentity(sourceDb, "regular-identity", false))
		require.Error(t, validateMigrationSourceDb(sourceDb))
	})

	t.Run("accepts a db that has a default admin identity", func(t *testing.T) {
		sourceDb, err := db.Open(filepath.Join(t.TempDir(), "populated.db"))
		require.NoError(t, err)
		defer func() { _ = sourceDb.Close() }()

		require.NoError(t, addIdentity(sourceDb, "regular-identity", false))
		require.NoError(t, addIdentity(sourceDb, "admin-identity", true))
		require.NoError(t, validateMigrationSourceDb(sourceDb))
	})
}

// addIdentity writes an identity bucket with the isDefaultAdmin field set, using boltz so the
// stored value is read back the same way the controller reads it.
func addIdentity(sourceDb boltz.Db, id string, isDefaultAdmin bool) error {
	return sourceDb.Update(nil, func(ctx boltz.MutateContext) error {
		idBucket := boltz.GetOrCreatePath(ctx.Tx(), db.RootBucket, db.EntityTypeIdentities, id)
		idBucket.SetBool(db.FieldIdentityIsDefaultAdmin, isDefaultAdmin, nil)
		return idBucket.GetError()
	})
}
