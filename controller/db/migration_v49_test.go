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
	"encoding/json"
	"testing"

	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
)

// Test_Migration_V49_AdoptsExistingL2ConfigTypes verifies that l2 config types created by
// a user before they became built-in, and so with non-standard ids, don't block the upgrade.
// They are updated in place, keeping their ids, and a type whose schema differs from the
// built-in one has its original schema saved under a "-replaced" name.
func Test_Migration_V49_AdoptsExistingL2ConfigTypes(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	userSchema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{"custom": map[string]interface{}{"type": "string"}},
	}

	hostId := eid.New()
	interceptId := eid.New()

	// Simulate a v48 datastore where a user created both l2 types with random ids.
	// l2.host.v1 gets a custom schema, l2.intercept.v1 gets one matching the built-in.
	err := ctx.GetDb().Update(nil, func(mc boltz.MutateContext) error {
		for _, id := range []string{l2HostV1ConfigType.Id, l2InterceptV1ConfigType.Id} {
			if err := ctx.stores.ConfigType.DeleteById(mc, id); err != nil {
				return err
			}
		}
		if err := ctx.stores.ConfigType.Create(mc, &ConfigType{
			BaseExtEntity: boltz.BaseExtEntity{Id: hostId},
			Name:          l2HostV1ConfigType.Name,
			Target:        ConfigTypeTargetService,
			Schema:        userSchema,
		}); err != nil {
			return err
		}
		if err := ctx.stores.ConfigType.Create(mc, &ConfigType{
			BaseExtEntity: boltz.BaseExtEntity{Id: interceptId},
			Name:          l2InterceptV1ConfigType.Name,
			Target:        ConfigTypeTargetService,
			Schema:        l2InterceptV1ConfigType.Schema,
		}); err != nil {
			return err
		}

		rootBucket := boltz.NewTypedBucket(nil, mc.Tx().Bucket([]byte(RootBucket)))
		versionsBucket := rootBucket.GetOrCreateBucket("versions")
		versionsBucket.SetInt64("edge", 48, nil)
		return versionsBucket.GetError()
	})
	ctx.NoError(err)

	ctx.NoError(RunMigrations(ctx.GetDb(), ctx.stores, nil))

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		host, err := ctx.stores.ConfigType.LoadOneByName(tx, l2HostV1ConfigType.Name)
		ctx.NoError(err)
		ctx.NotNil(host)
		ctx.Equal(hostId, host.Id, "existing id should be kept")
		ctx.Equal(schemaJson(ctx, l2HostV1ConfigType.Schema), schemaJson(ctx, host.Schema))

		backup, err := ctx.stores.ConfigType.LoadOneByName(tx, l2HostV1ConfigType.Name+"-replaced")
		ctx.NoError(err)
		ctx.NotNil(backup, "changed schema should be saved")
		ctx.NotEqual(hostId, backup.Id)
		ctx.Equal(schemaJson(ctx, userSchema), schemaJson(ctx, backup.Schema))

		intercept, err := ctx.stores.ConfigType.LoadOneByName(tx, l2InterceptV1ConfigType.Name)
		ctx.NoError(err)
		ctx.NotNil(intercept)
		ctx.Equal(interceptId, intercept.Id, "existing id should be kept")

		backup, err = ctx.stores.ConfigType.LoadOneByName(tx, l2InterceptV1ConfigType.Name+"-replaced")
		ctx.NoError(err)
		ctx.Nil(backup, "unchanged schema should not be saved")
		return nil
	})
	ctx.NoError(err)
}

func schemaJson(ctx *TestContext, schema map[string]interface{}) string {
	b, err := json.Marshal(schema)
	ctx.NoError(err)
	return string(b)
}
