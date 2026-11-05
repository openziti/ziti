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
	"fmt"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/common/version"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

// rerunChangesData lists the named migrations that change a datastore every time they run, with why
// that is acceptable.
var rerunChangesData = map[string]string{
	"session.clear-pre-1.7": "drops the legacy service sessions created since it last ran; clients request new ones",
}

// Test_NamedMigrationsRerunWithoutChange verifies that each named migration leaves a datastore it has
// already changed as it is, so a migration that runs again does no harm. A datastore holding each kind
// of entity the migrations touch stands in for one they have migrated.
func Test_NamedMigrationsRerunWithoutChange(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	attr := eid.New()
	identity := ctx.RequireNewIdentity(eid.New(), false)

	edgeService := newEdgeService(eid.New(), attr)
	boltztest.RequireCreate(ctx, edgeService)

	fabricService := newEdgeService(eid.New())
	fabricService.IsFabricOnly = true
	fabricService.EncryptionRequired = true
	boltztest.RequireCreate(ctx, fabricService)

	edgeRouter := newEdgeRouter(eid.New(), attr)
	boltztest.RequireCreate(ctx, edgeRouter)

	postureCheck := newProcessPostureCheck(nil, "")
	postureCheck.RoleAttributes = []string{attr}
	boltztest.RequireCreate(ctx, postureCheck)

	boltztest.RequireCreate(ctx, newConfigType(eid.New()))

	servicePolicy := newServicePolicy(eid.New())
	servicePolicy.IdentityRoles = ss(entityRef(identity.Id))
	servicePolicy.ServiceRoles = ss(roleRef(attr))
	servicePolicy.PostureCheckRoles = ss(roleRef(attr))
	boltztest.RequireCreate(ctx, servicePolicy)

	edgeRouterPolicy := newEdgeRouterPolicy(eid.New())
	edgeRouterPolicy.IdentityRoles = ss(entityRef(identity.Id))
	edgeRouterPolicy.EdgeRouterRoles = ss(roleRef(attr))
	boltztest.RequireCreate(ctx, edgeRouterPolicy)

	serp := newServiceEdgeRouterPolicy(eid.New())
	serp.ServiceRoles = ss(roleRef(attr))
	serp.EdgeRouterRoles = ss(AllRole)
	boltztest.RequireCreate(ctx, serp)

	before := dumpDatastore(t, ctx)

	for _, migration := range newMigrations(ctx.stores, nil).named {
		if _, changes := rerunChangesData[migration.Name]; changes {
			continue
		}
		ctx.NoError(ctx.GetDb().Update(change.New().NewMutateContext(), func(mctx boltz.MutateContext) error {
			step := &boltz.MigrationStep{Component: edgeComponent, Ctx: mctx, CurrentVersion: NamedMigrationsDbVersion}
			migration.Apply(step)
			return step.GetError()
		}), "migration %v", migration.Name)
	}

	ctx.Equal(before, dumpDatastore(t, ctx), "running the named migrations again should not change the datastore")
}

// Test_RunMigrationsFromNumberedVersion verifies that a datastore at a numbered version records the
// named migrations its numbered steps already ran without running them again, and runs the rest. The
// legacy session clear shows which happened: it was numbered step 44, and it removes a marker left in
// the sessions bucket when it runs.
func Test_RunMigrationsFromNumberedVersion(t *testing.T) {
	for _, dbVersion := range []int{43, 44, 47, maxLegacyDbVersion} {
		t.Run(fmt.Sprintf("version %d", dbVersion), func(t *testing.T) {
			ctx := NewTestContext(t)
			defer ctx.Cleanup()
			ctx.Init()

			ctx.NoError(ctx.GetDb().Update(nil, func(mctx boltz.MutateContext) error {
				root := mctx.Tx().Bucket([]byte(RootBucket))
				sessions, err := root.CreateBucketIfNotExists([]byte(EntityTypeSessions))
				if err != nil {
					return err
				}
				if err = sessions.Put([]byte("marker"), []byte("pre-1.7")); err != nil {
					return err
				}
				return resetToNumberedVersion(ctx, mctx.Tx(), dbVersion)
			}))

			ctx.NoError(RunMigrations(ctx.GetDb(), ctx.stores, nil))

			applied := requireAppliedMigrations(t, ctx)
			ctx.Equal(NamedMigrationsDbVersion, requireEdgeVersion(t, ctx))
			ctx.Len(applied, len(newMigrations(ctx.stores, nil).named), "every registered migration should be recorded")
			for _, migration := range newMigrations(ctx.stores, nil).named {
				record := applied[migration.Name]
				ctx.NotNil(record, "migration %v should be recorded", migration.Name)
				if numbered, ok := legacyNamedMigrations[migration.Name]; ok && numbered <= dbVersion {
					ctx.Equal(fmt.Sprintf("%s (edge datastore version %d)", version.GetVersion(), numbered), record.AppliedBy, migration.Name)
					ctx.True(record.AppliedAt.IsZero(), "when numbered step %d ran isn't known", numbered)
				} else {
					ctx.Equal(version.GetVersion(), record.AppliedBy, migration.Name)
					ctx.False(record.AppliedAt.IsZero(), migration.Name)
				}
			}
			ctx.Equal(dbVersion >= 44, sessionsMarkerPresent(t, ctx), "the session clear should run only below version 44")
		})
	}
}

// Test_RunMigrationsRefusesUnknownMigration verifies that a datastore recording a migration this
// controller doesn't have, because a newer controller migrated it, is refused without being changed.
func Test_RunMigrationsRefusesUnknownMigration(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	future := &namedMigration{Name: "future.migration", Version: "9.9"}
	ctx.NoError(ctx.GetDb().Update(nil, func(mctx boltz.MutateContext) error {
		return newMigrations(ctx.stores, nil).recordApplied(mctx.Tx(), future, "9.9.0", time.Now())
	}))
	before := dumpDatastore(t, ctx)

	err := RunMigrations(ctx.GetDb(), ctx.stores, nil)

	ctx.ErrorContains(err, `"future.migration"`)
	ctx.Equal(before, dumpDatastore(t, ctx), "a refused datastore should not be changed")
}

// Test_RunMigrationsRefusesUnsupportedVersion verifies that a numbered version above any this release
// line wrote is refused, since it came from a controller this one can't follow.
func Test_RunMigrationsRefusesUnsupportedVersion(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	ctx.NoError(ctx.GetDb().Update(nil, func(mctx boltz.MutateContext) error {
		return newMigrations(ctx.stores, nil).writeVersion(mctx.Tx(), maxLegacyDbVersion+1)
	}))

	ctx.ErrorContains(RunMigrations(ctx.GetDb(), ctx.stores, nil), "Unsupported edge datastore version")
}

// Test_RunMigrationsInitializesNewDatastore verifies that a new datastore records every named migration
// as applied, since initializing it produces what they migrate to.
func Test_RunMigrationsInitializesNewDatastore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	applied := requireAppliedMigrations(t, ctx)
	for _, migration := range newMigrations(ctx.stores, nil).named {
		record := applied[migration.Name]
		ctx.NotNil(record, "migration %v should be recorded", migration.Name)
		ctx.Equal(migration.Version, record.Version)
		ctx.Equal(version.GetVersion(), record.AppliedBy)
	}
	ctx.Equal(NamedMigrationsDbVersion, requireEdgeVersion(t, ctx))
}

func Test_MigrateRejectsDuplicateName(t *testing.T) {
	m := &Migrations{}
	m.migrate("test.migration", "2.1", nil)
	require.Panics(t, func() { m.migrate("test.migration", "2.1", nil) })
}

// resetToNumberedVersion makes the datastore look as it did at a numbered version, before named
// migrations: it removes the applied migration records and stores dbVersion as the edge version.
func resetToNumberedVersion(ctx *TestContext, tx *bbolt.Tx, dbVersion int) error {
	root := tx.Bucket([]byte(RootBucket))
	if root.Bucket([]byte(migrationsBucket)) != nil {
		if err := root.DeleteBucket([]byte(migrationsBucket)); err != nil {
			return err
		}
	}
	return newMigrations(ctx.stores, nil).writeVersion(tx, dbVersion)
}

// dumpDatastore returns every value under the root bucket, keyed by its quoted bucket path.
func dumpDatastore(t *testing.T, ctx *TestContext) map[string]string {
	result := map[string]string{}
	require.NoError(t, ctx.GetDb().View(func(tx *bbolt.Tx) error {
		return dumpBucket(tx.Bucket([]byte(RootBucket)), RootBucket, result)
	}))
	return result
}

func dumpBucket(bucket *bbolt.Bucket, path string, result map[string]string) error {
	return bucket.ForEach(func(key, value []byte) error {
		keyPath := fmt.Sprintf("%s/%q", path, key)
		if value == nil {
			return dumpBucket(bucket.Bucket(key), keyPath, result)
		}
		result[keyPath] = fmt.Sprintf("%q", value)
		return nil
	})
}

// requireAppliedMigrations returns the applied migration records, keyed by name.
func requireAppliedMigrations(t *testing.T, ctx *TestContext) map[string]*AppliedMigration {
	result := map[string]*AppliedMigration{}
	require.NoError(t, ctx.GetDb().View(func(tx *bbolt.Tx) error {
		records, err := LoadAppliedMigrations(tx)
		for _, record := range records {
			result[record.Name] = record
		}
		return err
	}))
	return result
}

func requireEdgeVersion(t *testing.T, ctx *TestContext) int {
	var result int
	require.NoError(t, ctx.GetDb().View(func(tx *bbolt.Tx) error {
		result = ReadEdgeVersion(tx)
		return nil
	}))
	return result
}

func sessionsMarkerPresent(t *testing.T, ctx *TestContext) bool {
	var present bool
	require.NoError(t, ctx.GetDb().View(func(tx *bbolt.Tx) error {
		if sessions := tx.Bucket([]byte(RootBucket)).Bucket([]byte(EntityTypeSessions)); sessions != nil {
			present = sessions.Get([]byte("marker")) != nil
		}
		return nil
	}))
	return present
}
