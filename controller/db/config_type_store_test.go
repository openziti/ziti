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
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/common/eid"
	"go.etcd.io/bbolt"
	"testing"
	"time"
)

func Test_ConfigTypeStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test config type CRUD", ctx.testConfigTypeCrud)
}

func (ctx *TestContext) testConfigTypeCrud(*testing.T) {
	ctx.CleanupAll()

	configType := newConfigType("")
	err := boltztest.Create(ctx, configType)
	ctx.EqualError(err, "index on configTypes.name does not allow null or empty values")

	configType = newConfigType(eid.New())
	boltztest.RequireCreate(ctx, configType)
	boltztest.ValidateBaseline(ctx, configType)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		testConfigType, err := ctx.stores.ConfigType.LoadOneByName(tx, configType.Name)
		ctx.NoError(err)
		ctx.NotNil(testConfigType)
		ctx.Equal(configType.Name, testConfigType.Name)

		return nil
	})
	ctx.NoError(err)

	time.Sleep(10 * time.Millisecond) // ensure updated time is different than created time
	configType.Name = eid.New()
	boltztest.RequireUpdate(ctx, configType)
	boltztest.ValidateUpdated(ctx, configType)

	config := newConfig(eid.New(), configType.Id, map[string]interface{}{
		"dnsHostname": "ssh.yourcompany.com",
		"port":        int64(22),
	})
	boltztest.RequireCreate(ctx, config)

	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ids := ctx.stores.ConfigType.GetRelatedEntitiesIdList(tx, configType.Id, EntityTypeConfigs)
		ctx.Equal([]string{config.Id}, ids)
		return nil
	})
	ctx.NoError(err)

	err = boltztest.Delete(ctx, configType)
	ctx.EqualError(err, fmt.Sprintf("cannot delete config type %v, as configs of that type exist", configType.Id))

	boltztest.RequireDelete(ctx, config)
	boltztest.RequireDelete(ctx, configType)
}
