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

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"go.etcd.io/bbolt"
)

// Test_Migration_V45_SetConfigTypeTargets verifies that re-running migrations from
// version 44 fills in target="service" on config types that have no target value,
// while leaving config types that already have a target unchanged.
func Test_Migration_V45_SetConfigTypeTargets(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	// Create one config type to be migrated (target will be cleared below to
	// simulate a pre-v45 row) and one that already has a non-default target,
	// which the migration should leave alone.
	needsMigration := newConfigType(eid.New())
	boltztest.RequireCreate(ctx, needsMigration)

	preserved := newConfigType(eid.New())
	preserved.Target = ConfigTypeTargetRouter
	boltztest.RequireCreate(ctx, preserved)

	// Simulate a pre-v45 datastore: remove the target value from `needsMigration`
	// and roll the edge component's version back to 44 so that re-running
	// migrations will exercise setConfigTypeTargets.
	err := ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
		bucket := ctx.stores.ConfigType.GetEntityBucket(mc.Tx(), []byte(needsMigration.Id))
		ctx.NotNil(bucket, "config type bucket should exist for %v", needsMigration.Id)
		bucket.DeleteValue([]byte(FieldConfigTypeTarget))
		if err := bucket.GetError(); err != nil {
			return err
		}

		rootBucket := boltz.NewTypedBucket(nil, mc.Tx().Bucket([]byte(RootBucket)))
		versionsBucket := rootBucket.GetOrCreateBucket("versions")
		versionsBucket.SetInt64("edge", 44, nil)
		return versionsBucket.GetError()
	})
	ctx.NoError(err)

	// Re-run migrations from v44 -> v45.
	ctx.NoError(RunMigrations(ctx.GetDb(), ctx.stores, nil))

	// Both config types should have a valid target. The previously-empty one
	// should now default to "service"; the other should keep its "router" value.
	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		loaded, loadErr := ctx.stores.ConfigType.LoadById(tx, needsMigration.Id)
		ctx.NoError(loadErr)
		ctx.Equal(ConfigTypeTargetService, loaded.Target)

		loaded, loadErr = ctx.stores.ConfigType.LoadById(tx, preserved.Id)
		ctx.NoError(loadErr)
		ctx.Equal(ConfigTypeTargetRouter, loaded.Target)
		return nil
	})
	ctx.NoError(err)
}
