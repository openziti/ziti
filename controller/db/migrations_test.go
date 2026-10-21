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
	"time"

	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
)

func Test_SyncConfigTypes(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("unchanged schemas are not rewritten", ctx.testSyncConfigTypesUnchanged)
	t.Run("changed schemas are updated", ctx.testSyncConfigTypesChanged)
	t.Run("missing config types are created", ctx.testSyncConfigTypesMissing)
}

func (ctx *TestContext) syncConfigTypes() error {
	m := &Migrations{stores: ctx.stores}
	return m.syncConfigTypes(ctx.GetDb())
}

func (ctx *TestContext) loadConfigType(name string) *ConfigType {
	var result *ConfigType
	ctx.NoError(ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		result, err = ctx.stores.ConfigType.LoadOneByName(tx, name)
		return err
	}))
	return result
}

func (ctx *TestContext) requireSchemaMatches(expected *ConfigType, actual *ConfigType) {
	expectedJson, err := json.Marshal(expected.Schema)
	ctx.NoError(err)
	actualJson, err := json.Marshal(actual.Schema)
	ctx.NoError(err)
	ctx.Equal(string(expectedJson), string(actualJson))
}

func (ctx *TestContext) testSyncConfigTypesUnchanged(*testing.T) {
	before := map[string]time.Time{}
	for _, cfgType := range syncedConfigTypes {
		stored := ctx.loadConfigType(cfgType.Name)
		ctx.NotNil(stored, "config type %s should have been created by migrations", cfgType.Name)
		ctx.requireSchemaMatches(cfgType, stored)
		before[cfgType.Name] = stored.UpdatedAt
	}

	time.Sleep(10 * time.Millisecond) // ensure any update would produce a different updated time
	ctx.NoError(ctx.syncConfigTypes())

	for _, cfgType := range syncedConfigTypes {
		stored := ctx.loadConfigType(cfgType.Name)
		ctx.Equal(before[cfgType.Name], stored.UpdatedAt, "config type %s should not have been updated", cfgType.Name)
	}
}

func (ctx *TestContext) testSyncConfigTypesChanged(*testing.T) {
	// simulate a config type stored by an earlier 2.0.x release, which lacks a newer property
	stored := ctx.loadConfigType(hostV1ConfigType.Name)
	stored.Schema = map[string]interface{}{"type": "object"}
	stored.Tags = map[string]interface{}{"foo": "bar"}
	mutateCtx := change.New().NewMutateContext()
	ctx.NoError(ctx.GetDb().Update(mutateCtx, func(mutateCtx boltz.MutateContext) error {
		return ctx.stores.ConfigType.Update(mutateCtx, stored, nil)
	}))
	before := ctx.loadConfigType(hostV1ConfigType.Name).UpdatedAt

	time.Sleep(10 * time.Millisecond)
	ctx.NoError(ctx.syncConfigTypes())

	updated := ctx.loadConfigType(hostV1ConfigType.Name)
	ctx.requireSchemaMatches(hostV1ConfigType, updated)
	ctx.True(updated.UpdatedAt.After(before))
	ctx.Equal("bar", updated.Tags["foo"], "fields other than schema should be preserved")
}

func (ctx *TestContext) testSyncConfigTypesMissing(*testing.T) {
	mutateCtx := change.New().NewMutateContext()
	ctx.NoError(ctx.GetDb().Update(mutateCtx, func(mutateCtx boltz.MutateContext) error {
		return ctx.stores.ConfigType.DeleteById(mutateCtx, l2HostV1ConfigType.Id)
	}))
	ctx.Nil(ctx.loadConfigType(l2HostV1ConfigType.Name))

	ctx.NoError(ctx.syncConfigTypes())

	created := ctx.loadConfigType(l2HostV1ConfigType.Name)
	ctx.NotNil(created)
	ctx.Equal(l2HostV1ConfigType.Id, created.Id)
	ctx.requireSchemaMatches(l2HostV1ConfigType, created)
}
