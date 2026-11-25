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

// sessionsMarkerKey is a key putSessionsMarker leaves in the sessions bucket, which only the pre-1.7
// session clear removes.
const sessionsMarkerKey = "marker"

// Test_NamedMigrationsRerunWithoutChange verifies that each named migration leaves a datastore it has
// already changed as it is, so a migration that runs again does no harm. A datastore holding each kind
// of entity the migrations touch stands in for one they have migrated.
func Test_NamedMigrationsRerunWithoutChange(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	attr := eid.New()
	identity := newIdentity(eid.New(), "")
	ctx.NoError(boltztest.Create(ctx, identity))
	ctx.NoError(boltztest.Create(ctx, newEdgeService(eid.New(), attr)))
	ctx.NoError(boltztest.Create(ctx, newEdgeRouter(eid.New(), attr)))
	postureCheck := newProcessPostureCheck(nil, "")
	postureCheck.RoleAttributes = []string{attr}
	ctx.NoError(boltztest.Create(ctx, postureCheck))
	ctx.NoError(boltztest.Create(ctx, newConfigType(eid.New())))
	servicePolicy := newServicePolicy(eid.New())
	servicePolicy.IdentityRoles = ss(entityRef(identity.Id))
	servicePolicy.ServiceRoles = ss(roleRef(attr))
	servicePolicy.PostureCheckRoles = ss(roleRef(attr))
	ctx.NoError(boltztest.Create(ctx, servicePolicy))
	edgeRouterPolicy := newEdgeRouterPolicy(eid.New())
	edgeRouterPolicy.IdentityRoles = ss(entityRef(identity.Id))
	edgeRouterPolicy.EdgeRouterRoles = ss(roleRef(attr))
	ctx.NoError(boltztest.Create(ctx, edgeRouterPolicy))
	serp := newServiceEdgeRouterPolicy(eid.New())
	serp.ServiceRoles = ss(roleRef(attr))
	serp.EdgeRouterRoles = ss(AllRole)
	ctx.NoError(boltztest.Create(ctx, serp))
	before, err := dumpDatastore(ctx)
	ctx.NoError(err)

	for _, migration := range newMigrations(ctx.stores, nil).named {
		if _, changes := rerunChangesData[migration.Name]; changes {
			continue
		}
		err = ctx.GetDb().Update(change.New().NewMutateContext(), func(mctx boltz.MutateContext) error {
			step := &boltz.MigrationStep{Component: edgeComponent, Ctx: mctx, CurrentVersion: NamedMigrationsDbVersion}
			migration.Apply(step)
			return step.GetError()
		})
		ctx.NoError(err, "migration %v", migration.Name)
	}

	after, err := dumpDatastore(ctx)
	ctx.NoError(err)
	ctx.Equal(before, after, "running the named migrations again should not change the datastore")
}

// Test_RunMigrationsFromNumberedVersion verifies that a datastore at a numbered version records the
// named migrations its numbered steps already ran without running them again, and runs the rest. The
// legacy session clear shows which happened: it was numbered step 44, and it removes a marker left in
// the sessions bucket when it runs.
func Test_RunMigrationsFromNumberedVersion(t *testing.T) {
	sessionClearVersion := legacyNamedMigrations["session.clear-pre-1.7"]

	for _, dbVersion := range []int{43, MaxLegacyDbVersion} {
		t.Run(fmt.Sprintf("version %d", dbVersion), func(t *testing.T) {
			ctx := NewTestContext(t)
			defer ctx.Cleanup()
			ctx.NoError(putSessionsMarker(ctx))
			ctx.NoError(resetToNumberedVersion(ctx, dbVersion))

			err := RunMigrations(ctx.GetDb(), ctx.stores, nil)

			ctx.NoError(err)
			edgeVersion, err := loadEdgeVersion(ctx)
			ctx.NoError(err)
			ctx.Equal(NamedMigrationsDbVersion, edgeVersion, "a converted datastore should store the named migrations version")
			applied, err := loadAppliedMigrations(ctx)
			ctx.NoError(err)
			named := newMigrations(ctx.stores, nil).named
			ctx.Len(applied, len(named), "every registered migration should be recorded")
			for _, migration := range named {
				record := applied[migration.Name]
				ctx.NotNil(record, "migration %v should be recorded", migration.Name)
				if numbered, ok := legacyNamedMigrations[migration.Name]; ok && numbered <= dbVersion {
					ctx.Equal(fmt.Sprintf("%s (edge datastore version %d)", version.GetVersion(), numbered), record.AppliedBy,
						"%v ran as numbered step %d, so it should be recorded as applied by that step", migration.Name, numbered)
					ctx.True(record.AppliedAt.IsZero(), "when numbered step %d ran isn't known", numbered)
				} else {
					ctx.Equal(version.GetVersion(), record.AppliedBy, "%v should be recorded as applied by this controller", migration.Name)
					ctx.False(record.AppliedAt.IsZero(), "%v should record when it ran", migration.Name)
				}
			}
			markerPresent, err := hasSessionsMarker(ctx)
			ctx.NoError(err)
			ctx.Equal(dbVersion >= sessionClearVersion, markerPresent,
				"the session clear should run only on a datastore below version %d", sessionClearVersion)
		})
	}
}

// Test_RunMigrationsRefusesUnknownMigration verifies that a datastore recording a migration this
// controller doesn't have, because a newer controller migrated it, is refused without being changed.
func Test_RunMigrationsRefusesUnknownMigration(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	future := &namedMigration{Name: "future.migration", Version: "9.9"}
	ctx.NoError(ctx.GetDb().Update(nil, func(mctx boltz.MutateContext) error {
		return newMigrations(ctx.stores, nil).recordApplied(mctx.Tx(), future, "9.9.0", time.Now())
	}))
	before, err := dumpDatastore(ctx)
	ctx.NoError(err)

	err = RunMigrations(ctx.GetDb(), ctx.stores, nil)

	ctx.ErrorContains(err, `"future.migration"`, "the error should name the migration this controller doesn't have")
	after, err := dumpDatastore(ctx)
	ctx.NoError(err)
	ctx.Equal(before, after, "a refused datastore should not be changed")
}

// Test_RunMigrationsRefusesUnsupportedVersion verifies that a numbered version above any this release
// line wrote is refused, since it came from a controller this one can't follow.
func Test_RunMigrationsRefusesUnsupportedVersion(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.NoError(resetToNumberedVersion(ctx, MaxLegacyDbVersion+1))

	err := RunMigrations(ctx.GetDb(), ctx.stores, nil)

	ctx.ErrorContains(err, "Unsupported edge datastore version")
}

// Test_RunMigrationsInitializesNewDatastore verifies that a new datastore records every named migration
// as applied, since initializing it produces what they migrate to.
func Test_RunMigrationsInitializesNewDatastore(t *testing.T) {
	// the test context initializes a new datastore
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	applied, err := loadAppliedMigrations(ctx)
	ctx.NoError(err)
	for _, migration := range newMigrations(ctx.stores, nil).named {
		record := applied[migration.Name]
		ctx.NotNil(record, "a new datastore should record %v as applied", migration.Name)
		ctx.Equal(migration.Version, record.Version, migration.Name)
		ctx.Equal(version.GetVersion(), record.AppliedBy, migration.Name)
	}
	edgeVersion, err := loadEdgeVersion(ctx)
	ctx.NoError(err)
	ctx.Equal(NamedMigrationsDbVersion, edgeVersion, "a new datastore should store the named migrations version")
}

// Test_MigrateRejectsDuplicateName verifies that registering two migrations under one name panics, since
// a datastore records each migration by its name.
func Test_MigrateRejectsDuplicateName(t *testing.T) {
	m := &Migrations{}
	m.migrate("test.migration", "2.1", nil)

	require.Panics(t, func() { m.migrate("test.migration", "2.1", nil) })
}

// resetToNumberedVersion makes the datastore look as it did at a numbered version, before named
// migrations: it removes the applied migration records and stores dbVersion as the edge version.
func resetToNumberedVersion(ctx *TestContext, dbVersion int) error {
	return ctx.GetDb().Update(nil, func(mctx boltz.MutateContext) error {
		root := mctx.Tx().Bucket([]byte(RootBucket))
		if root.Bucket([]byte(migrationsBucket)) != nil {
			if err := root.DeleteBucket([]byte(migrationsBucket)); err != nil {
				return err
			}
		}
		return newMigrations(ctx.stores, nil).writeVersion(mctx.Tx(), dbVersion)
	})
}

// putSessionsMarker stores sessionsMarkerKey in the sessions bucket, creating the bucket if needed.
func putSessionsMarker(ctx *TestContext) error {
	return ctx.GetDb().Update(nil, func(mctx boltz.MutateContext) error {
		root := mctx.Tx().Bucket([]byte(RootBucket))
		sessions, err := root.CreateBucketIfNotExists([]byte(EntityTypeSessions))
		if err != nil {
			return err
		}
		return sessions.Put([]byte(sessionsMarkerKey), []byte("pre-1.7"))
	})
}

// hasSessionsMarker reports whether the sessions bucket still holds sessionsMarkerKey.
func hasSessionsMarker(ctx *TestContext) (bool, error) {
	var present bool
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		if sessions := tx.Bucket([]byte(RootBucket)).Bucket([]byte(EntityTypeSessions)); sessions != nil {
			present = sessions.Get([]byte(sessionsMarkerKey)) != nil
		}
		return nil
	})
	return present, err
}

// dumpDatastore returns every value under the root bucket, keyed by its quoted bucket path.
func dumpDatastore(ctx *TestContext) (map[string]string, error) {
	result := map[string]string{}
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		return dumpBucket(tx.Bucket([]byte(RootBucket)), RootBucket, result)
	})
	return result, err
}

// dumpBucket adds every value under bucket to result, keyed by its quoted path below path.
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

// loadAppliedMigrations returns the applied migration records, keyed by name.
func loadAppliedMigrations(ctx *TestContext) (map[string]*AppliedMigration, error) {
	result := map[string]*AppliedMigration{}
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		records, err := LoadAppliedMigrations(tx)
		for _, record := range records {
			result[record.Name] = record
		}
		return err
	})
	return result, err
}

// loadEdgeVersion returns the stored edge datastore version.
func loadEdgeVersion(ctx *TestContext) (int, error) {
	var result int
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		result = ReadEdgeVersion(tx)
		return nil
	})
	return result, err
}
