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
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/xt"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func Test_MigrateCollapseEdgeServices(t *testing.T) {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())

	const (
		fabricOnlyId   = "svc-fabric-only"
		fabricOnlyName = "fabric-service"

		edgeFullId   = "svc-edge-full"
		edgeFullName = "edge-service-full"

		edgeMinimalId   = "svc-edge-minimal"
		edgeMinimalName = "edge-service-minimal"
	)

	// Open a fresh DB
	dbPath := filepath.Join(t.TempDir(), "test.db")
	boltDb, err := Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = boltDb.Close() }()

	// Write pre-migration (pre-collapse) data and roll the edge version back so the collapse migration runs
	err = boltDb.Update(nil, func(ctx boltz.MutateContext) error {
		tx := ctx.Tx()

		// Roll the edge version back to a pre-collapse value so the collapse migration runs
		versionsBucket := boltz.GetOrCreatePath(tx, RootBucket, "versions")
		if versionsBucket.HasError() {
			return versionsBucket.GetError()
		}
		versionsBucket.SetInt64("edge", 44, nil)
		if versionsBucket.HasError() {
			return versionsBucket.GetError()
		}

		servicesBucket, err := tx.Bucket([]byte(RootBucket)).CreateBucketIfNotExists([]byte(EntityTypeServices))
		if err != nil {
			return err
		}
		_ = servicesBucket

		// 1. Fabric-only service: no "edge" sub-bucket
		if err := writePreMigrationService(t, tx, fabricOnlyId, fabricOnlyName, nil); err != nil {
			return err
		}

		// 2. Edge service (full): has role attributes and encryption required
		if err := writePreMigrationService(t, tx, edgeFullId, edgeFullName, func(edgeBucket *bbolt.Bucket) error {
			// Set name in edge bucket (duplicated from parent, as the old store did)
			// Use type-prefixed encoding as boltz TypedBucket would
			if err := edgeBucket.Put([]byte(FieldName), boltz.PrependFieldType(boltz.TypeString, []byte(edgeFullName))); err != nil {
				return err
			}

			// Set encryptionRequired = true (already uses encodeBool which adds type prefix)
			if err := edgeBucket.Put([]byte(FieldServiceEncryptionRequired), encodeBool(true)); err != nil {
				return err
			}

			// Set role attributes as a set bucket (keys are type-prefixed strings)
			rolesBucket, err := edgeBucket.CreateBucket([]byte(FieldRoleAttributes))
			if err != nil {
				return err
			}
			if err := rolesBucket.Put(boltz.PrependFieldType(boltz.TypeString, []byte("web")), nil); err != nil {
				return err
			}
			if err := rolesBucket.Put(boltz.PrependFieldType(boltz.TypeString, []byte("database")), nil); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return err
		}

		// 3. Edge service (minimal): edge sub-bucket with only encryptionRequired=false, no role attributes
		if err := writePreMigrationService(t, tx, edgeMinimalId, edgeMinimalName, func(edgeBucket *bbolt.Bucket) error {
			if err := edgeBucket.Put([]byte(FieldName), boltz.PrependFieldType(boltz.TypeString, []byte(edgeMinimalName))); err != nil {
				return err
			}
			return edgeBucket.Put([]byte(FieldServiceEncryptionRequired), encodeBool(false))
		}); err != nil {
			return err
		}

		// 4. The pre-collapse role-attributes index, as the old edge service store maintained it:
		// ziti/indexes/services/roleAttributes/<value>/<type-prefixed id>. The old edge child store
		// inherited its parent's entity type, so the index already lives at the unified service
		// path (verified against a real pre-collapse db); the migration leaves it untouched.
		rolesIndexBucket := boltz.GetOrCreatePath(tx, RootBucket, boltz.IndexesBucket, EntityTypeServices, FieldRoleAttributes)
		if rolesIndexBucket.HasError() {
			return rolesIndexBucket.GetError()
		}
		for _, attr := range []string{"web", "database"} {
			valueBucket := rolesIndexBucket.GetOrCreateBucket(attr)
			if valueBucket.HasError() {
				return valueBucket.GetError()
			}
			if valueBucket.SetListEntry(boltz.TypeString, []byte(edgeFullId)).HasError() {
				return valueBucket.GetError()
			}
		}

		// 5. The name unique index for all three services, as the fabric service store maintained
		// it pre-collapse: ziti/indexes/services/name/<name> = <id>.
		nameIndexBucket := boltz.GetOrCreatePath(tx, RootBucket, boltz.IndexesBucket, EntityTypeServices, FieldName)
		if nameIndexBucket.HasError() {
			return nameIndexBucket.GetError()
		}
		for id, name := range map[string]string{
			fabricOnlyId:  fabricOnlyName,
			edgeFullId:    edgeFullName,
			edgeMinimalId: edgeMinimalName,
		} {
			if err := nameIndexBucket.Bucket.Put([]byte(name), []byte(id)); err != nil {
				return err
			}
		}

		return nil
	})
	require.NoError(t, err)

	// Verify pre-migration state: edge sub-buckets exist
	err = boltDb.View(func(tx *bbolt.Tx) error {
		servicesBucket := boltz.Path(tx, RootBucket, EntityTypeServices)
		require.NotNil(t, servicesBucket)

		// Fabric-only should have no edge bucket
		fabricBucket := servicesBucket.GetBucket(fabricOnlyId)
		require.NotNil(t, fabricBucket)
		require.Nil(t, fabricBucket.GetBucket(EdgeBucket))

		// Edge full should have edge bucket
		edgeFullBucket := servicesBucket.GetBucket(edgeFullId)
		require.NotNil(t, edgeFullBucket)
		require.NotNil(t, edgeFullBucket.GetBucket(EdgeBucket))

		// Edge minimal should have edge bucket
		edgeMinBucket := servicesBucket.GetBucket(edgeMinimalId)
		require.NotNil(t, edgeMinBucket)
		require.NotNil(t, edgeMinBucket.GetBucket(EdgeBucket))

		return nil
	})
	require.NoError(t, err)

	// The migration must report no anomaly events: the indexes are already in place (nothing for
	// the integrity check to fix) and no edge-owned fields exist at the service level (no
	// pre-existing destinations to replace).
	var collapseEvents []string
	ServiceCollapseEventListener = func(event string) {
		collapseEvents = append(collapseEvents, event)
	}
	defer func() { ServiceCollapseEventListener = nil }()

	// Run InitStores, which runs all pending migrations including the collapse
	stores, err := InitStores(boltDb, command.NoOpRateLimiter{}, nil)
	require.NoError(t, err)
	require.Empty(t, collapseEvents, "migration should report no anomaly events")

	closeNotify := make(chan struct{})
	defer close(closeNotify)
	require.NoError(t, stores.EventualEventer.Start(closeNotify))

	// Verify post-migration state
	err = boltDb.View(func(tx *bbolt.Tx) error {
		// 1. Fabric-only service should be IsFabricOnly=true
		t.Run("fabric-only service has IsFabricOnly=true", func(t *testing.T) {
			svc, _, err := stores.Service.FindById(tx, fabricOnlyId)
			require.NoError(t, err)
			require.NotNil(t, svc)
			require.True(t, svc.IsFabricOnly)
			require.Equal(t, fabricOnlyName, svc.Name)
			require.True(t, svc.EncryptionRequired) // migration writes the secure default, matching the fabric create path
			require.Nil(t, svc.RoleAttributes)
		})

		// 2. Edge full service should be IsFabricOnly=false with fields migrated
		t.Run("edge service (full) has fields migrated", func(t *testing.T) {
			svc, _, err := stores.Service.FindById(tx, edgeFullId)
			require.NoError(t, err)
			require.NotNil(t, svc)
			require.False(t, svc.IsFabricOnly)
			require.Equal(t, edgeFullName, svc.Name)
			require.True(t, svc.EncryptionRequired)
			require.ElementsMatch(t, []string{"web", "database"}, svc.RoleAttributes)
		})

		// 3. Edge minimal service should be IsFabricOnly=false with encryptionRequired=false
		t.Run("edge service (minimal) has fields migrated", func(t *testing.T) {
			svc, _, err := stores.Service.FindById(tx, edgeMinimalId)
			require.NoError(t, err)
			require.NotNil(t, svc)
			require.False(t, svc.IsFabricOnly)
			require.Equal(t, edgeMinimalName, svc.Name)
			require.False(t, svc.EncryptionRequired)
			require.Nil(t, svc.RoleAttributes)
		})

		// 4. Edge sub-buckets should be removed
		t.Run("edge sub-buckets are removed", func(t *testing.T) {
			servicesBucket := boltz.Path(tx, RootBucket, EntityTypeServices)
			require.NotNil(t, servicesBucket)

			edgeFullBucket := servicesBucket.GetBucket(edgeFullId)
			require.NotNil(t, edgeFullBucket)
			require.Nil(t, edgeFullBucket.GetBucket(EdgeBucket))

			edgeMinBucket := servicesBucket.GetBucket(edgeMinimalId)
			require.NotNil(t, edgeMinBucket)
			require.Nil(t, edgeMinBucket.GetBucket(EdgeBucket))
		})

		// 5. IsFabricOnly filtering works
		t.Run("isFabricOnly filtering works", func(t *testing.T) {
			ids, _, err := stores.Service.QueryIds(tx, `isFabricOnly = true`)
			require.NoError(t, err)
			require.Contains(t, ids, fabricOnlyId)
			require.NotContains(t, ids, edgeFullId)
			require.NotContains(t, ids, edgeMinimalId)

			ids, _, err = stores.Service.QueryIds(tx, `isFabricOnly = false`)
			require.NoError(t, err)
			require.NotContains(t, ids, fabricOnlyId)
			require.Contains(t, ids, edgeFullId)
			require.Contains(t, ids, edgeMinimalId)
		})

		// 6. Role attribute queries work on migrated data
		t.Run("role attribute queries work", func(t *testing.T) {
			ids, _, err := stores.Service.QueryIds(tx, `anyOf(roleAttributes) = "web"`)
			require.NoError(t, err)
			require.Contains(t, ids, edgeFullId)
			require.NotContains(t, ids, fabricOnlyId)
			require.NotContains(t, ids, edgeMinimalId)
		})

		return nil
	})
	require.NoError(t, err)

	// 7. CheckIntegrity(fix=false) reports no drift after migration (M7).
	t.Run("CheckIntegrity is clean post-migration", func(t *testing.T) {
		var integrityErrs []error
		err := stores.CheckIntegrity(boltDb, context.Background(), false, func(e error, _ bool) {
			integrityErrs = append(integrityErrs, e)
		})
		require.NoError(t, err)
		require.Empty(t, integrityErrs, "post-migration integrity check should report no drift")
	})

	// 8. The migration is idempotent / version-gated: re-running migrations on the already-migrated DB is a
	// no-op and does not corrupt the already-migrated data (M8).
	t.Run("migration is idempotent on re-run", func(t *testing.T) {
		require.NoError(t, RunMigrations(boltDb, stores, nil))

		err := boltDb.View(func(tx *bbolt.Tx) error {
			fabric, _, err := stores.Service.FindById(tx, fabricOnlyId)
			require.NoError(t, err)
			require.True(t, fabric.IsFabricOnly)
			require.True(t, fabric.EncryptionRequired)
			require.Nil(t, fabric.RoleAttributes)

			edge, _, err := stores.Service.FindById(tx, edgeFullId)
			require.NoError(t, err)
			require.False(t, edge.IsFabricOnly)
			require.True(t, edge.EncryptionRequired)
			require.ElementsMatch(t, []string{"web", "database"}, edge.RoleAttributes)

			// no edge sub-bucket should have reappeared
			servicesBucket := boltz.Path(tx, RootBucket, EntityTypeServices)
			require.Nil(t, servicesBucket.GetBucket(edgeFullId).GetBucket(EdgeBucket))
			return nil
		})
		require.NoError(t, err)
	})
}

// writePreMigrationService writes a service entity in the pre-migration format.
// Uses boltz TypedBucket for proper encoding of base entity fields.
// The edgeDataFn is called to populate the "edge" sub-bucket; if nil, no edge bucket is created (fabric-only).
func writePreMigrationService(t *testing.T, tx *bbolt.Tx, id, name string, edgeDataFn func(*bbolt.Bucket) error) error {
	t.Helper()

	entityBucket := boltz.GetOrCreatePath(tx, RootBucket, EntityTypeServices, id)
	if entityBucket.HasError() {
		return entityBucket.GetError()
	}

	// Write base entity fields using TypedBucket for proper encoding
	now := time.Now()
	entityBucket.SetString(FieldName, name, nil)
	entityBucket.SetString(FieldServiceTerminatorStrategy, "smartrouting", nil)
	entityBucket.SetTimeP(boltz.FieldCreatedAt, &now, nil)
	entityBucket.SetTimeP(boltz.FieldUpdatedAt, &now, nil)
	if entityBucket.HasError() {
		return entityBucket.GetError()
	}

	if edgeDataFn != nil {
		rawBucket := tx.Bucket([]byte(RootBucket)).Bucket([]byte(EntityTypeServices)).Bucket([]byte(id))
		edgeBucket, err := rawBucket.CreateBucket([]byte(EdgeBucket))
		if err != nil {
			return err
		}
		return edgeDataFn(edgeBucket)
	}

	return nil
}

// Test_MigrateCollapseEdgeServices_PreExistingDestinations covers the anomalous case where
// edge-owned fields already exist at the service level pre-collapse. Such data was unreachable
// through the pre-collapse stores, so the migration replaces it with the authoritative edge data
// and reports each replacement through ServiceCollapseEventListener.
func Test_MigrateCollapseEdgeServices_PreExistingDestinations(t *testing.T) {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())

	const svcId = "svc-conflicted"
	const svcName = "conflicted-service"

	dbPath := filepath.Join(t.TempDir(), "test.db")
	boltDb, err := Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = boltDb.Close() }()

	err = boltDb.Update(nil, func(ctx boltz.MutateContext) error {
		tx := ctx.Tx()

		versionsBucket := boltz.GetOrCreatePath(tx, RootBucket, "versions")
		versionsBucket.SetInt64("edge", 44, nil)
		if versionsBucket.HasError() {
			return versionsBucket.GetError()
		}

		if err := writePreMigrationService(t, tx, svcId, svcName, func(edgeBucket *bbolt.Bucket) error {
			if err := edgeBucket.Put([]byte(FieldName), boltz.PrependFieldType(boltz.TypeString, []byte(svcName))); err != nil {
				return err
			}
			if err := edgeBucket.Put([]byte(FieldServiceEncryptionRequired), encodeBool(false)); err != nil {
				return err
			}
			rolesBucket, err := edgeBucket.CreateBucket([]byte(FieldRoleAttributes))
			if err != nil {
				return err
			}
			return rolesBucket.Put(boltz.PrependFieldType(boltz.TypeString, []byte("web")), nil)
		}); err != nil {
			return err
		}

		// The anomaly: service-level data colliding with edge-owned fields (a scalar and a
		// bucket), unreachable through the pre-collapse stores
		entityBucket := tx.Bucket([]byte(RootBucket)).Bucket([]byte(EntityTypeServices)).Bucket([]byte(svcId))
		if err := entityBucket.Put([]byte(FieldServiceEncryptionRequired), encodeBool(true)); err != nil {
			return err
		}
		staleRoles, err := entityBucket.CreateBucket([]byte(FieldRoleAttributes))
		if err != nil {
			return err
		}
		if err := staleRoles.Put(boltz.PrependFieldType(boltz.TypeString, []byte("stale")), nil); err != nil {
			return err
		}

		// The real pre-collapse index state: name index, and the role-attributes index
		// reflecting the edge data (which is what the old stores maintained)
		nameIndexBucket := boltz.GetOrCreatePath(tx, RootBucket, boltz.IndexesBucket, EntityTypeServices, FieldName)
		if nameIndexBucket.HasError() {
			return nameIndexBucket.GetError()
		}
		if err := nameIndexBucket.Bucket.Put([]byte(svcName), []byte(svcId)); err != nil {
			return err
		}
		rolesIndexBucket := boltz.GetOrCreatePath(tx, RootBucket, boltz.IndexesBucket, EntityTypeServices, FieldRoleAttributes)
		if rolesIndexBucket.HasError() {
			return rolesIndexBucket.GetError()
		}
		valueBucket := rolesIndexBucket.GetOrCreateBucket("web")
		if valueBucket.HasError() {
			return valueBucket.GetError()
		}
		valueBucket.SetListEntry(boltz.TypeString, []byte(svcId))
		return valueBucket.GetError()
	})
	require.NoError(t, err)

	var collapseEvents []string
	ServiceCollapseEventListener = func(event string) {
		collapseEvents = append(collapseEvents, event)
	}
	defer func() { ServiceCollapseEventListener = nil }()

	stores, err := InitStores(boltDb, command.NoOpRateLimiter{}, nil)
	require.NoError(t, err)

	closeNotify := make(chan struct{})
	defer close(closeNotify)
	require.NoError(t, stores.EventualEventer.Start(closeNotify))

	// Each replaced destination is reported, and nothing else (no integrity events)
	require.Len(t, collapseEvents, 2, "expected exactly the two replacement events, got: %v", collapseEvents)
	joined := strings.Join(collapseEvents, "\n")
	require.Contains(t, joined, `"`+FieldServiceEncryptionRequired+`"`)
	require.Contains(t, joined, `"`+FieldRoleAttributes+`"`)
	require.Contains(t, joined, svcId)

	// The edge data won
	err = boltDb.View(func(tx *bbolt.Tx) error {
		svc, _, err := stores.Service.FindById(tx, svcId)
		require.NoError(t, err)
		require.NotNil(t, svc)
		require.False(t, svc.EncryptionRequired)
		require.ElementsMatch(t, []string{"web"}, svc.RoleAttributes)
		return nil
	})
	require.NoError(t, err)
}
