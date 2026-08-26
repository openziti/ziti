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

	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/xeipuuv/gojsonschema"
	"go.etcd.io/bbolt"
)

// Test_Migration_V48_RefreshesRouterLinkV1Schema verifies that a controller
// created at version 47, which stored router.link.v1 with the stricter
// channelOptions minimums, has its persisted schema refreshed on upgrade.
//
// Validation reads the schema out of the database rather than the Go literal,
// so without this migration an upgraded controller would keep rejecting the
// zero queue sizes that a fresh install accepts.
func Test_Migration_V48_RefreshesRouterLinkV1Schema(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	// Simulate a v47 datastore: overwrite the stored schema with the stricter
	// pre-v48 shape and roll the edge component's version back to 47.
	err := ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
		stored, loadErr := ctx.stores.ConfigType.LoadOneByName(mc.Tx(), RouterLinkV1TypeId)
		if loadErr != nil {
			return loadErr
		}
		ctx.NotNil(stored, "router.link.v1 must be registered as a built-in")

		channelOptions := stored.Schema["definitions"].(map[string]interface{})["channelOptions"].(map[string]interface{})
		props := channelOptions["properties"].(map[string]interface{})
		props["outQueueSize"].(map[string]interface{})["minimum"] = 1
		props["maxQueuedConnects"].(map[string]interface{})["minimum"] = 1

		if err := ctx.stores.ConfigType.Update(mc, stored, nil); err != nil {
			return err
		}

		rootBucket := boltz.NewTypedBucket(nil, mc.Tx().Bucket([]byte(RootBucket)))
		versionsBucket := rootBucket.GetOrCreateBucket("versions")
		versionsBucket.SetInt64("edge", 47, nil)
		return versionsBucket.GetError()
	})
	ctx.NoError(err)

	// Confirm the rollback actually took, so a passing test can't be an artifact
	// of the stored schema having been permissive all along.
	ctx.False(storedSchemaAcceptsZeroQueueSizes(ctx), "precondition: v47 schema should reject zero queue sizes")

	ctx.NoError(RunMigrations(ctx.GetDb(), ctx.stores, nil))

	ctx.True(storedSchemaAcceptsZeroQueueSizes(ctx), "migration should refresh the persisted schema to accept zero queue sizes")

	// The relaxation is limited to the two buffer sizes; the worker count still
	// needs a positive floor.
	ctx.False(storedSchemaAccepts(ctx, map[string]interface{}{
		"listeners": []interface{}{
			map[string]interface{}{
				"bind":    "tls:0.0.0.0:6262",
				"options": map[string]interface{}{"maxOutstandingConnects": 0},
			},
		},
	}), "maxOutstandingConnects must still require at least 1")
}

func storedSchemaAcceptsZeroQueueSizes(ctx *TestContext) bool {
	return storedSchemaAccepts(ctx, map[string]interface{}{
		"listeners": []interface{}{
			map[string]interface{}{
				"bind": "tls:0.0.0.0:6262",
				"options": map[string]interface{}{
					"outQueueSize":      0,
					"maxQueuedConnects": 0,
				},
			},
		},
	})
}

// storedSchemaAccepts validates payload against the router.link.v1 schema as it
// is currently persisted, mirroring how config creation validates at runtime.
func storedSchemaAccepts(ctx *TestContext, payload map[string]interface{}) bool {
	var stored *ConfigType
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var loadErr error
		stored, loadErr = ctx.stores.ConfigType.LoadOneByName(tx, RouterLinkV1TypeId)
		return loadErr
	})
	ctx.NoError(err)
	ctx.NotNil(stored)

	schema, err := gojsonschema.NewSchemaLoader().Compile(gojsonschema.NewGoLoader(stored.Schema))
	ctx.NoError(err)

	result, err := schema.Validate(gojsonschema.NewGoLoader(payload))
	ctx.NoError(err)
	return result.Valid()
}
