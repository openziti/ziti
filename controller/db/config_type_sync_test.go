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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/command"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

// systemConfigTypeSchemaHashes maps each system config type name and SystemVersion to the sha256 of
// the JSON-marshalled schema. A SystemVersion names one schema on every release line, so entries are
// added on main when a version is bumped, copied verbatim to release branches, and never edited.
var systemConfigTypeSchemaHashes = map[string]map[int32]string{
	"ziti-tunneler-client.v1": {1: "6d91e7c87d8469315decb40e75812a66f340dfa3fd2c37d4125703acac8baa70"},
	"ziti-tunneler-server.v1": {1: "e57286768cfcc68002cad1b9da935ede755f84de320068983b8a9c56c3fe5540"},
	"intercept.v1":            {1: "57d98133462607ee9286098cce6ca9a99a8e0b0610788c4be353b6b937fbfa69"},
	"host.v1":                 {1: "c89ca6f42b4afbd3985eaa5d83720f57e47ee0df87aae2f3eabfa413d3271136"},
	"host.v2":                 {1: "47712003a8264a9411a32967131e4207c0bd6264f73ce6c019f6c9692d0037f6"},
	"interfaces.v1":           {1: "7d5ae9af00d1e00529e99fcf3376b8913258ea2906bfecbd6f4513decba30761"},
	"host-interfaces.v1":      {1: "8c424964351f7db2d2dc888925322827ac4601f7f073f16e39bce0d813d43f55"},
	"proxy.v1":                {1: "12cc3cf3c456ef42287376a01535e8471d7fe49981da2407c357cab110aec458"},
	"router.link.v1":          {1: "1638bf2f6c2c1d38093b8f459a6388f8c5ed624361bd0fc5b7278d0ca8875cde"},
	"l2.host.v1":              {1: "96ed34e80ed0eeef577c2721fa2f96cbb9480f177bb13459f82c074bfbac6c98"},
	"l2.intercept.v1":         {1: "19ac9bdf84b845056e7e61fbbc51dac9cc5486228bf6e45539ba3329e6637ce0"},
}

func schemaJson(t require.TestingT, schema map[string]interface{}) string {
	result, err := json.Marshal(schema)
	require.NoError(t, err)
	return string(result)
}

func schemaHash(t require.TestingT, schema map[string]interface{}) string {
	sum := sha256.Sum256([]byte(schemaJson(t, schema)))
	return hex.EncodeToString(sum[:])
}

func Test_SystemConfigTypeDefinitions(t *testing.T) {
	t.Run("names and ids are unique", func(t *testing.T) {
		names := map[string]struct{}{}
		ids := map[string]struct{}{}
		for _, def := range systemConfigTypes {
			require.NotContains(t, names, def.Name)
			require.NotContains(t, ids, def.Id)
			names[def.Name] = struct{}{}
			ids[def.Id] = struct{}{}
		}
	})

	t.Run("versions are valid", func(t *testing.T) {
		for _, def := range systemConfigTypes {
			require.Positive(t, def.SystemVersion, def.Name)
		}
	})

	t.Run("schemas match the hash recorded for their version", func(t *testing.T) {
		for _, def := range systemConfigTypes {
			actual := schemaHash(t, def.Schema)
			recorded, found := systemConfigTypeSchemaHashes[def.Name][def.SystemVersion]
			require.True(t, found, "no schema hash recorded for %s version %d. If this is a new version, record %s",
				def.Name, def.SystemVersion, actual)
			require.Equal(t, recorded, actual, "schema of %s differs from the one recorded for version %d. "+
				"Bump its SystemVersion and record %s for the new version", def.Name, def.SystemVersion, actual)
		}
	})
}

func Test_SyncSystemConfigTypes(t *testing.T) {
	staleSchema := map[string]interface{}{"type": "object"}

	t.Run("creates every system config type", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		for _, def := range systemConfigTypes {
			stored := loadConfigTypeByName(ctx, def.Name)
			ctx.NotNil(stored, def.Name)
			ctx.Equal(def.Id, stored.Id)
			ctx.Equal(def.SystemVersion, stored.SystemVersion)
			ctx.Equal(schemaJson(t, def.Schema), schemaJson(t, stored.Schema))
		}
	})

	t.Run("does not rewrite current types", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		updatedAt := map[string]time.Time{}
		for _, def := range systemConfigTypes {
			updatedAt[def.Name] = loadConfigTypeByName(ctx, def.Name).UpdatedAt
		}

		time.Sleep(10 * time.Millisecond) // so a rewrite would change UpdatedAt
		ctx.NoError(syncConfigTypes(ctx))

		for _, def := range systemConfigTypes {
			ctx.Equal(updatedAt[def.Name], loadConfigTypeByName(ctx, def.Name).UpdatedAt, def.Name)
		}
	})

	t.Run("updates a type stored at a lower version", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		replaceStoredConfigType(ctx, hostV2ConfigType.Name, func(stored *ConfigType) {
			stored.Schema = staleSchema
			stored.SystemVersion = 0
		})

		ctx.NoError(syncConfigTypes(ctx))

		stored := loadConfigTypeByName(ctx, hostV2ConfigType.Name)
		ctx.Equal(hostV2ConfigType.Id, stored.Id)
		ctx.Equal(hostV2ConfigType.SystemVersion, stored.SystemVersion)
		ctx.Equal(schemaJson(t, hostV2ConfigType.Schema), schemaJson(t, stored.Schema))
	})

	t.Run("adopts a type created under a different id", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		customId := eid.New()
		ctx.NoError(ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
			if err := ctx.stores.ConfigType.DeleteById(mc, interfacesConfigTypeV1.Id); err != nil {
				return err
			}
			return ctx.stores.ConfigType.Create(mc, &ConfigType{
				BaseExtEntity: boltz.BaseExtEntity{Id: customId},
				Name:          interfacesConfigTypeV1.Name,
				Schema:        staleSchema,
			})
		}))

		ctx.NoError(syncConfigTypes(ctx))

		stored := loadConfigTypeByName(ctx, interfacesConfigTypeV1.Name)
		ctx.Equal(customId, stored.Id)
		ctx.Equal(interfacesConfigTypeV1.SystemVersion, stored.SystemVersion)
		ctx.Equal(schemaJson(t, interfacesConfigTypeV1.Schema), schemaJson(t, stored.Schema))
	})

	t.Run("leaves types at the same or a higher version alone", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		replaceStoredConfigType(ctx, hostV1ConfigType.Name, func(stored *ConfigType) {
			stored.Schema = staleSchema
		})
		replaceStoredConfigType(ctx, interceptV1ConfigType.Name, func(stored *ConfigType) {
			stored.Schema = staleSchema
			stored.SystemVersion = interceptV1ConfigType.SystemVersion + 1
		})

		ctx.NoError(syncConfigTypes(ctx))

		stored := loadConfigTypeByName(ctx, hostV1ConfigType.Name)
		ctx.Equal(hostV1ConfigType.SystemVersion, stored.SystemVersion)
		ctx.Equal(schemaJson(t, staleSchema), schemaJson(t, stored.Schema))

		stored = loadConfigTypeByName(ctx, interceptV1ConfigType.Name)
		ctx.Equal(interceptV1ConfigType.SystemVersion+1, stored.SystemVersion)
		ctx.Equal(schemaJson(t, staleSchema), schemaJson(t, stored.Schema))
	})

	t.Run("renames a renamed type back and updates it", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		replaceStoredConfigType(ctx, interfacesConfigTypeV1.Name, func(stored *ConfigType) {
			stored.Name = "renamed-interfaces"
			stored.Schema = staleSchema
			stored.SystemVersion = 0
		})

		ctx.NoError(syncConfigTypes(ctx))

		ctx.Nil(loadConfigTypeByName(ctx, "renamed-interfaces"))
		stored := loadConfigTypeByName(ctx, interfacesConfigTypeV1.Name)
		ctx.NotNil(stored)
		ctx.Equal(interfacesConfigTypeV1.Id, stored.Id)
		ctx.Equal(interfacesConfigTypeV1.SystemVersion, stored.SystemVersion)
		ctx.Equal(schemaJson(t, interfacesConfigTypeV1.Schema), schemaJson(t, stored.Schema))
	})

	t.Run("renaming back leaves a newer schema alone", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		replaceStoredConfigType(ctx, interfacesConfigTypeV1.Name, func(stored *ConfigType) {
			stored.Name = "renamed-interfaces"
			stored.Schema = staleSchema
			stored.SystemVersion = interfacesConfigTypeV1.SystemVersion + 1
		})

		ctx.NoError(syncConfigTypes(ctx))

		stored := loadConfigTypeByName(ctx, interfacesConfigTypeV1.Name)
		ctx.NotNil(stored)
		ctx.Equal(interfacesConfigTypeV1.Id, stored.Id)
		ctx.Equal(interfacesConfigTypeV1.SystemVersion+1, stored.SystemVersion)
		ctx.Equal(schemaJson(t, staleSchema), schemaJson(t, stored.Schema))
	})

	t.Run("recreates a deleted type", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		ctx.NoError(ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
			return ctx.stores.ConfigType.DeleteById(mc, interfacesConfigTypeV1.Id)
		}))
		ctx.Nil(loadConfigTypeByName(ctx, interfacesConfigTypeV1.Name))

		ctx.NoError(syncConfigTypes(ctx))

		stored := loadConfigTypeByName(ctx, interfacesConfigTypeV1.Name)
		ctx.NotNil(stored)
		ctx.Equal(interfacesConfigTypeV1.Id, stored.Id)
		ctx.Equal(interfacesConfigTypeV1.SystemVersion, stored.SystemVersion)
	})

	t.Run("updates limited to api fields keep the stored version", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		// mirrors the fields ConfigTypeManager allows an API update to change
		apiFields := boltz.MapFieldChecker{
			FieldName:             struct{}{},
			FieldConfigTypeSchema: struct{}{},
			boltz.FieldTags:       struct{}{},
		}

		stored := loadConfigTypeByName(ctx, hostV1ConfigType.Name)
		stored.Schema = staleSchema
		stored.SystemVersion = 0
		ctx.NoError(ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
			return ctx.stores.ConfigType.Update(mc, stored, apiFields)
		}))

		stored = loadConfigTypeByName(ctx, hostV1ConfigType.Name)
		ctx.Equal(schemaJson(t, staleSchema), schemaJson(t, stored.Schema))
		ctx.Equal(hostV1ConfigType.SystemVersion, stored.SystemVersion)
	})
}

func Test_RunMigrationsSnapshots(t *testing.T) {
	staleSchema := map[string]interface{}{"type": "object"}

	makeStale := func(ctx *TestContext, names ...string) {
		for _, name := range names {
			replaceStoredConfigType(ctx, name, func(stored *ConfigType) {
				stored.Schema = staleSchema
				stored.SystemVersion = 0
			})
		}
	}

	// requireSnapshotIsStale checks that the snapshot at path holds the named types as makeStale left them
	requireSnapshotIsStale := func(t *testing.T, ctx *TestContext, path string, names ...string) {
		snapshot, err := bbolt.Open(path, 0600, &bbolt.Options{ReadOnly: true})
		require.NoError(t, err)
		defer func() { _ = snapshot.Close() }()

		require.NoError(t, snapshot.View(func(tx *bbolt.Tx) error {
			for _, name := range names {
				stored, err := ctx.stores.ConfigType.LoadOneByName(tx, name)
				require.NoError(t, err)
				require.NotNil(t, stored, name)
				require.Zero(t, stored.SystemVersion, name)
				require.Equal(t, schemaJson(t, staleSchema), schemaJson(t, stored.Schema), name)
			}
			return nil
		}))
	}

	requireCurrent := func(t *testing.T, ctx *TestContext, defs ...*ConfigType) {
		for _, def := range defs {
			stored := loadConfigTypeByName(ctx, def.Name)
			require.Equal(t, def.SystemVersion, stored.SystemVersion, def.Name)
			require.Equal(t, schemaJson(t, def.Schema), schemaJson(t, stored.Schema), def.Name)
		}
	}

	t.Run("does not snapshot when nothing changes", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		db := newSnapshotCountingDb(t, ctx.GetDb())
		require.NoError(t, RunMigrations(db, ctx.stores, nil))
		require.Zero(t, db.snapshots)
	})

	t.Run("does not snapshot a new datastore", func(t *testing.T) {
		boltDb, err := Open(filepath.Join(t.TempDir(), "ctrl.db"))
		require.NoError(t, err)
		defer func() { _ = boltDb.Close() }()

		db := newSnapshotCountingDb(t, boltDb)
		_, err = InitStores(db, command.NoOpRateLimiter{}, nil)
		require.NoError(t, err)
		require.Zero(t, db.snapshots)
	})

	t.Run("snapshots once before syncing when the db version is current", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		makeStale(ctx, hostV2ConfigType.Name, interfacesConfigTypeV1.Name)

		db := newSnapshotCountingDb(t, ctx.GetDb())
		require.NoError(t, RunMigrations(db, ctx.stores, nil))
		require.Equal(t, 1, db.snapshots)

		requireSnapshotIsStale(t, ctx, db.path, hostV2ConfigType.Name, interfacesConfigTypeV1.Name)
		requireCurrent(t, ctx, hostV2ConfigType, interfacesConfigTypeV1)
	})

	t.Run("relies on the version migration snapshot", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		makeStale(ctx, hostV2ConfigType.Name)
		ctx.NoError(ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
			root := boltz.NewTypedBucket(nil, mc.Tx().Bucket([]byte(RootBucket)))
			versions := root.GetOrCreateBucket("versions")
			versions.SetInt64("edge", CurrentDbVersion-1, nil)
			return versions.GetError()
		}))

		db := newSnapshotCountingDb(t, ctx.GetDb())
		require.NoError(t, RunMigrations(db, ctx.stores, nil))
		require.Equal(t, 1, db.snapshots)

		requireSnapshotIsStale(t, ctx, db.path, hostV2ConfigType.Name)
		requireCurrent(t, ctx, hostV2ConfigType)
	})

	t.Run("does not write when the snapshot fails", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		makeStale(ctx, hostV2ConfigType.Name)

		db := newSnapshotCountingDb(t, ctx.GetDb())
		db.path = filepath.Join(t.TempDir(), "missing", "snapshot.db")
		require.ErrorContains(t, RunMigrations(db, ctx.stores, nil), "failed to create bolt db snapshot")

		stored := loadConfigTypeByName(ctx, hostV2ConfigType.Name)
		require.Zero(t, stored.SystemVersion)
		require.Equal(t, schemaJson(t, staleSchema), schemaJson(t, stored.Schema))
		_, err := os.Stat(db.path)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

// snapshotCountingDb directs snapshots to a fixed path in a test temp dir and counts them.
type snapshotCountingDb struct {
	boltz.Db
	path      string
	snapshots int
}

func newSnapshotCountingDb(t *testing.T, db boltz.Db) *snapshotCountingDb {
	return &snapshotCountingDb{
		Db:   db,
		path: filepath.Join(t.TempDir(), "snapshot.db"),
	}
}

func (db *snapshotCountingDb) GetDefaultSnapshotPath() string {
	return db.path
}

func (db *snapshotCountingDb) SnapshotInTx(tx *bbolt.Tx, path string) (string, string, error) {
	db.snapshots++
	return db.Db.SnapshotInTx(tx, path)
}

func syncConfigTypes(ctx *TestContext) error {
	m := &Migrations{stores: ctx.stores}
	return m.syncSystemConfigTypes(ctx.GetDb(), false)
}

func loadConfigTypeByName(ctx *TestContext, name string) *ConfigType {
	var result *ConfigType
	ctx.NoError(ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		result, err = ctx.stores.ConfigType.LoadOneByName(tx, name)
		return err
	}))
	return result
}

// replaceStoredConfigType loads the named config type, applies f, and writes every field back.
func replaceStoredConfigType(ctx *TestContext, name string, f func(stored *ConfigType)) {
	stored := loadConfigTypeByName(ctx, name)
	ctx.NotNil(stored, name)
	f(stored)
	ctx.NoError(ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
		return ctx.stores.ConfigType.Update(mc, stored, nil)
	}))
}
