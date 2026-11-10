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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
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

// staleSchema stands in for an outdated system config type schema.
var staleSchema = map[string]interface{}{"type": "object"}

func Test_SystemConfigTypeDefinitions(t *testing.T) {
	t.Run("names and ids are unique", func(t *testing.T) {
		names := map[string]struct{}{}
		ids := map[string]struct{}{}

		for _, def := range systemConfigTypes {
			require.NotContains(t, names, def.Name, "two system config types are named %s", def.Name)
			require.NotContains(t, ids, def.Id, "%s reuses the id of another system config type", def.Name)
			names[def.Name] = struct{}{}
			ids[def.Id] = struct{}{}
		}
	})

	t.Run("versions are valid", func(t *testing.T) {
		for _, def := range systemConfigTypes {
			require.Positive(t, def.SystemVersion,
				"%s needs a SystemVersion above 0, the version of user-defined and unsynced types", def.Name)
		}
	})

	// A SystemVersion names one schema everywhere. If a schema changes without a version bump, or a
	// release line carries a different schema under the same version, this fails.
	t.Run("schemas match the hash recorded for their version", func(t *testing.T) {
		for _, def := range systemConfigTypes {
			schemaJson, err := json.Marshal(def.Schema)
			require.NoError(t, err, def.Name)
			sum := sha256.Sum256(schemaJson)
			actual := hex.EncodeToString(sum[:])

			recorded, found := systemConfigTypeSchemaHashes[def.Name][def.SystemVersion]
			require.True(t, found, "no schema hash recorded for %s version %d. If this is a new version, record %s",
				def.Name, def.SystemVersion, actual)
			require.Equal(t, recorded, actual, "schema of %s differs from the one recorded for version %d. "+
				"Bump its SystemVersion and record %s for the new version", def.Name, def.SystemVersion, actual)
		}
	})
}

func Test_SyncSystemConfigTypes(t *testing.T) {
	t.Run("creates every system config type", func(t *testing.T) {
		// the test context runs the migrations on a new datastore, which syncs the system config types
		ctx := NewTestContext(t)
		defer ctx.Cleanup()

		for _, def := range systemConfigTypes {
			stored, err := loadConfigTypeByName(ctx, def.Name)
			ctx.NoError(err)
			ctx.NotNil(stored, "%s should be created on a new datastore", def.Name)
			ctx.Equal(def.Id, stored.Id, "%s should be created under its fixed id", def.Name)
			ctx.Equal(def.SystemVersion, stored.SystemVersion, def.Name)
			requireSchemaEqual(t, def.Schema, stored.Schema, def.Name)
		}
	})

	t.Run("does not rewrite current types", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		writes := &configTypeWriteRecorder{}
		ctx.stores.ConfigType.AddEntityEventListener(writes, boltz.EntityCreated, boltz.EntityUpdated)

		migrations := &Migrations{stores: ctx.stores}
		err := migrations.syncSystemConfigTypes(ctx.GetDb(), false)

		ctx.NoError(err)
		ctx.Empty(writes.names, "every system config type is current, so the sync should write none of them")
	})

	t.Run("updates a type stored at a lower version", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		ctx.NoError(makeConfigTypeStale(ctx, hostV2ConfigType.Name))
		writes := &configTypeWriteRecorder{}
		ctx.stores.ConfigType.AddEntityEventListener(writes, boltz.EntityCreated, boltz.EntityUpdated)

		migrations := &Migrations{stores: ctx.stores}
		err := migrations.syncSystemConfigTypes(ctx.GetDb(), false)

		ctx.NoError(err)
		ctx.Equal([]string{hostV2ConfigType.Name}, writes.names, "only the stale type should be written")
		stored, err := loadConfigTypeByName(ctx, hostV2ConfigType.Name)
		ctx.NoError(err)
		ctx.Equal(hostV2ConfigType.Id, stored.Id)
		ctx.Equal(hostV2ConfigType.SystemVersion, stored.SystemVersion)
		requireSchemaEqual(t, hostV2ConfigType.Schema, stored.Schema, "the stale schema should be replaced")
	})

	t.Run("adopts a type created under a different id", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		customId := eid.New()
		ctx.NoError(boltztest.Delete(ctx, proxyConfigTypeV1))
		ctx.NoError(boltztest.Create(ctx, &ConfigType{
			BaseExtEntity: boltz.BaseExtEntity{Id: customId},
			Name:          proxyConfigTypeV1.Name,
			Schema:        staleSchema,
		}))

		migrations := &Migrations{stores: ctx.stores}
		err := migrations.syncSystemConfigTypes(ctx.GetDb(), false)

		ctx.NoError(err)
		stored, err := loadConfigTypeByName(ctx, proxyConfigTypeV1.Name)
		ctx.NoError(err)
		ctx.Equal(customId, stored.Id, "the type should keep its id, since configs may already reference it")
		ctx.Equal(proxyConfigTypeV1.SystemVersion, stored.SystemVersion)
		requireSchemaEqual(t, proxyConfigTypeV1.Schema, stored.Schema, "the stale schema should be replaced")
	})

	t.Run("leaves types at the same or a higher version alone", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		sameVersion, err := loadConfigTypeByName(ctx, hostV1ConfigType.Name)
		ctx.NoError(err)
		sameVersion.Schema = staleSchema
		ctx.NoError(boltztest.Update(ctx, sameVersion))
		higherVersion, err := loadConfigTypeByName(ctx, interceptV1ConfigType.Name)
		ctx.NoError(err)
		higherVersion.Schema = staleSchema
		higherVersion.SystemVersion = interceptV1ConfigType.SystemVersion + 1
		ctx.NoError(boltztest.Update(ctx, higherVersion))
		writes := &configTypeWriteRecorder{}
		ctx.stores.ConfigType.AddEntityEventListener(writes, boltz.EntityCreated, boltz.EntityUpdated)

		migrations := &Migrations{stores: ctx.stores}
		err = migrations.syncSystemConfigTypes(ctx.GetDb(), false)

		ctx.NoError(err)
		ctx.Empty(writes.names, "a stored version at or above the definition's means the schema is current")
		stored, err := loadConfigTypeByName(ctx, hostV1ConfigType.Name)
		ctx.NoError(err)
		ctx.Equal(hostV1ConfigType.SystemVersion, stored.SystemVersion)
		requireSchemaEqual(t, staleSchema, stored.Schema, "the schema is trusted at the definition's version")
		stored, err = loadConfigTypeByName(ctx, interceptV1ConfigType.Name)
		ctx.NoError(err)
		ctx.Equal(interceptV1ConfigType.SystemVersion+1, stored.SystemVersion, "the sync should never downgrade")
		requireSchemaEqual(t, staleSchema, stored.Schema, "the sync should never downgrade")
	})

	t.Run("renames a renamed type back and updates it", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		renamed, err := loadConfigTypeByName(ctx, proxyConfigTypeV1.Name)
		ctx.NoError(err)
		renamed.Name = "renamed-proxy"
		renamed.Schema = staleSchema
		renamed.SystemVersion = 0
		ctx.NoError(boltztest.Update(ctx, renamed))

		migrations := &Migrations{stores: ctx.stores}
		err = migrations.syncSystemConfigTypes(ctx.GetDb(), false)

		ctx.NoError(err)
		stored, err := loadConfigTypeByName(ctx, "renamed-proxy")
		ctx.NoError(err)
		ctx.Nil(stored, "the type should no longer be stored under the name it was renamed to")
		stored, err = loadConfigTypeByName(ctx, proxyConfigTypeV1.Name)
		ctx.NoError(err)
		ctx.NotNil(stored, "the type should be found by its fixed id and renamed back")
		ctx.Equal(proxyConfigTypeV1.Id, stored.Id)
		ctx.Equal(proxyConfigTypeV1.SystemVersion, stored.SystemVersion)
		requireSchemaEqual(t, proxyConfigTypeV1.Schema, stored.Schema, "the stale schema should be replaced")
	})

	t.Run("renaming back leaves a newer schema alone", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		renamed, err := loadConfigTypeByName(ctx, proxyConfigTypeV1.Name)
		ctx.NoError(err)
		renamed.Name = "renamed-proxy"
		renamed.Schema = staleSchema
		renamed.SystemVersion = proxyConfigTypeV1.SystemVersion + 1
		ctx.NoError(boltztest.Update(ctx, renamed))

		migrations := &Migrations{stores: ctx.stores}
		err = migrations.syncSystemConfigTypes(ctx.GetDb(), false)

		ctx.NoError(err)
		stored, err := loadConfigTypeByName(ctx, proxyConfigTypeV1.Name)
		ctx.NoError(err)
		ctx.NotNil(stored, "the type should be found by its fixed id and renamed back")
		ctx.Equal(proxyConfigTypeV1.Id, stored.Id)
		ctx.Equal(proxyConfigTypeV1.SystemVersion+1, stored.SystemVersion, "renaming should not downgrade")
		requireSchemaEqual(t, staleSchema, stored.Schema, "renaming should not downgrade")
	})

	t.Run("recreates a deleted type", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		ctx.NoError(boltztest.Delete(ctx, l2InterceptV1ConfigType))

		migrations := &Migrations{stores: ctx.stores}
		err := migrations.syncSystemConfigTypes(ctx.GetDb(), false)

		ctx.NoError(err)
		stored, err := loadConfigTypeByName(ctx, l2InterceptV1ConfigType.Name)
		ctx.NoError(err)
		ctx.NotNil(stored, "a deleted system config type should be created again")
		ctx.Equal(l2InterceptV1ConfigType.Id, stored.Id, "a recreated type should get its fixed id")
		ctx.Equal(l2InterceptV1ConfigType.SystemVersion, stored.SystemVersion)
	})

	// API updates can't carry the hidden system version, so the store must not write it unless asked.
	t.Run("an update that doesn't name the system version keeps it", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		stored, err := loadConfigTypeByName(ctx, hostV1ConfigType.Name)
		ctx.NoError(err)
		stored.Schema = staleSchema
		stored.SystemVersion = 0

		err = boltztest.Patch(ctx, stored, boltz.MapFieldChecker{FieldConfigTypeSchema: struct{}{}})

		ctx.NoError(err)
		stored, err = loadConfigTypeByName(ctx, hostV1ConfigType.Name)
		ctx.NoError(err)
		requireSchemaEqual(t, staleSchema, stored.Schema, "the update names the schema, so it should be written")
		ctx.Equal(hostV1ConfigType.SystemVersion, stored.SystemVersion,
			"the update doesn't name the system version, so the stored one should be kept")
	})
}

func Test_RunMigrationsSnapshots(t *testing.T) {
	t.Run("does not snapshot when nothing changes", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		db := newSnapshotCountingDb(t, ctx.GetDb())

		err := RunMigrations(db, ctx.stores, nil)

		ctx.NoError(err)
		ctx.Zero(db.snapshots, "the db version and every config type are current, so nothing needs a snapshot")
	})

	t.Run("does not snapshot a new datastore", func(t *testing.T) {
		boltDb, err := Open(filepath.Join(t.TempDir(), "ctrl.db"))
		require.NoError(t, err)
		defer func() { _ = boltDb.Close() }()
		db := newSnapshotCountingDb(t, boltDb)

		_, err = InitStores(db, command.NoOpRateLimiter{}, nil)

		require.NoError(t, err)
		require.Zero(t, db.snapshots, "a new datastore has nothing to preserve")
	})

	t.Run("snapshots once before syncing when the db version is current", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		ctx.NoError(makeConfigTypeStale(ctx, hostV2ConfigType.Name))
		ctx.NoError(makeConfigTypeStale(ctx, proxyConfigTypeV1.Name))
		db := newSnapshotCountingDb(t, ctx.GetDb())

		err := RunMigrations(db, ctx.stores, nil)

		ctx.NoError(err)
		ctx.Equal(1, db.snapshots, "the sync should snapshot once, before its first write")
		for _, def := range []*ConfigType{hostV2ConfigType, proxyConfigTypeV1} {
			snapshotted, err := loadConfigTypeFromSnapshot(ctx, db.path, def.Name)
			ctx.NoError(err)
			ctx.NotNil(snapshotted, def.Name)
			ctx.Zero(snapshotted.SystemVersion, "the snapshot should hold %s as it was before the sync", def.Name)
			requireSchemaEqual(t, staleSchema, snapshotted.Schema,
				"the snapshot should hold %s as it was before the sync", def.Name)

			stored, err := loadConfigTypeByName(ctx, def.Name)
			ctx.NoError(err)
			ctx.Equal(def.SystemVersion, stored.SystemVersion, def.Name)
			requireSchemaEqual(t, def.Schema, stored.Schema, def.Name)
		}
	})

	t.Run("relies on the version migration snapshot", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		ctx.NoError(makeConfigTypeStale(ctx, hostV2ConfigType.Name))
		// roll the edge db version back one, so the version migrations run and take their snapshot
		ctx.NoError(ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
			root := boltz.NewTypedBucket(nil, mc.Tx().Bucket([]byte(RootBucket)))
			versions := root.GetOrCreateBucket("versions")
			versions.SetInt64("edge", CurrentDbVersion-1, nil)
			return versions.GetError()
		}))
		db := newSnapshotCountingDb(t, ctx.GetDb())

		err := RunMigrations(db, ctx.stores, nil)

		ctx.NoError(err)
		ctx.Equal(1, db.snapshots, "the version migration snapshot covers the sync, so the sync shouldn't take another")
		snapshotted, err := loadConfigTypeFromSnapshot(ctx, db.path, hostV2ConfigType.Name)
		ctx.NoError(err)
		ctx.NotNil(snapshotted)
		ctx.Zero(snapshotted.SystemVersion, "the snapshot should hold the type as it was before the sync")
		requireSchemaEqual(t, staleSchema, snapshotted.Schema, "the snapshot should hold the type as it was before the sync")
		stored, err := loadConfigTypeByName(ctx, hostV2ConfigType.Name)
		ctx.NoError(err)
		ctx.Equal(hostV2ConfigType.SystemVersion, stored.SystemVersion)
		requireSchemaEqual(t, hostV2ConfigType.Schema, stored.Schema)
	})

	t.Run("does not write when the snapshot fails", func(t *testing.T) {
		ctx := NewTestContext(t)
		defer ctx.Cleanup()
		ctx.NoError(makeConfigTypeStale(ctx, hostV2ConfigType.Name))
		db := newSnapshotCountingDb(t, ctx.GetDb())
		db.path = filepath.Join(t.TempDir(), "missing", "snapshot.db")

		err := RunMigrations(db, ctx.stores, nil)

		ctx.ErrorContains(err, "failed to create bolt db snapshot")
		stored, err := loadConfigTypeByName(ctx, hostV2ConfigType.Name)
		ctx.NoError(err)
		ctx.Zero(stored.SystemVersion, "a failed snapshot should leave the stale type unwritten")
		requireSchemaEqual(t, staleSchema, stored.Schema, "a failed snapshot should leave the stale type unwritten")
		_, err = os.Stat(db.path)
		ctx.ErrorIs(err, os.ErrNotExist)
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

// configTypeWriteRecorder records the name of each config type the store creates or updates. The store
// calls it synchronously on commit, so names is complete once the writing call returns.
type configTypeWriteRecorder struct {
	names []string
}

func (self *configTypeWriteRecorder) HandleEntityEvent(entity *ConfigType) {
	self.names = append(self.names, entity.Name)
}

// loadConfigTypeByName returns the stored config type with the given name, or nil if there is none.
func loadConfigTypeByName(ctx *TestContext, name string) (*ConfigType, error) {
	var result *ConfigType
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		result, err = ctx.stores.ConfigType.LoadOneByName(tx, name)
		return err
	})
	return result, err
}

// loadConfigTypeFromSnapshot returns the config type with the given name from the datastore snapshot at
// path, or nil if the snapshot has none.
func loadConfigTypeFromSnapshot(ctx *TestContext, path, name string) (*ConfigType, error) {
	snapshot, err := bbolt.Open(path, 0600, &bbolt.Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = snapshot.Close() }()

	var result *ConfigType
	err = snapshot.View(func(tx *bbolt.Tx) error {
		var err error
		result, err = ctx.stores.ConfigType.LoadOneByName(tx, name)
		return err
	})
	return result, err
}

// makeConfigTypeStale rewrites the stored config type with the given name to staleSchema at
// SystemVersion 0.
func makeConfigTypeStale(ctx *TestContext, name string) error {
	stored, err := loadConfigTypeByName(ctx, name)
	if err != nil {
		return err
	}
	if stored == nil {
		return fmt.Errorf("config type %s not found", name)
	}
	stored.Schema = staleSchema
	stored.SystemVersion = 0
	return boltztest.Update(ctx, stored)
}

// requireSchemaEqual fails the test unless expected and actual marshal to equivalent JSON. A schema
// read back from the store holds JSON-decoded values, not the Go types of a definition's literals, so
// the two can't be compared directly.
func requireSchemaEqual(t *testing.T, expected, actual map[string]interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	expectedJson, err := json.Marshal(expected)
	require.NoError(t, err)
	actualJson, err := json.Marshal(actual)
	require.NoError(t, err)
	require.JSONEq(t, string(expectedJson), string(actualJson), msgAndArgs...)
}
