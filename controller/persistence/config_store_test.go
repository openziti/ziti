/*
	Copyright 2020 Netfoundry, Inc.

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

package persistence

import (
	"github.com/google/uuid"
	"go.etcd.io/bbolt"
	"testing"
	"time"
)

func Test_ConfigStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test config CRUD", ctx.testConfigCrud)
	t.Run("test config Query", ctx.testConfigQuery)
}

func (ctx *TestContext) testConfigCrud(*testing.T) {
	ctx.cleanupAll()

	config := newConfig(uuid.New().String(), map[string]interface{}{
		"dnsHostname" : "ssh.yourcompany.com",
		"port" : int64(22),
	})
	ctx.requireCreate(config)
	ctx.validateBaseline(config)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		testConfig, err := ctx.stores.Config.LoadOneByName(tx, config.Name)
		ctx.NoError(err)
		ctx.NotNil(testConfig)
		ctx.Equal(config.Name, testConfig.Name)

		return nil
	})
	ctx.NoError(err)

	config = newConfig(uuid.New().String(), map[string]interface{}{
		"dnsHostname" : "ssh.yourcompany.com",
		"port" : int64(22),
		"enabled" : true,
		"nested" : map[string]interface{}{
			"hello":    "hi",
			"fromage?": "that's cheese",
			"count":    1000.32,
		},
	})
	ctx.requireCreate(config)
	ctx.validateBaseline(config)

	config = newConfig(uuid.New().String(), map[string]interface{}{
		"dnsHostname" : "ssh.yourcompany.com",
		"port" : int64(22),
		"enabled" : true,
		"nested" : map[string]interface{}{
			"hello":    "hi",
			"fromage?": "that's cheese",
			"count":    1000.32,
			"how" : map[string]interface{}{
				"nested" : map[string]interface{}{
					"can" : "it be?",
					"beep" : int64(2),
					"bop" : false,
				},
			},
		},
	})
	ctx.requireCreate(config)
	ctx.validateBaseline(config)

	config.Data = map[string]interface{}{
		"dnsHostname": "ssh.mycompany.com",
		"support":        int64(22),
	}

	time.Sleep(10 * time.Millisecond) // ensure updated time is different than created time
	ctx.requireUpdate(config)
	ctx.validateUpdated(config)

	ctx.requireDelete(config)
}

func (ctx *TestContext) testConfigQuery(*testing.T) {
	ctx.cleanupAll()

	config := newConfig(uuid.New().String(), map[string]interface{}{
		"dnsHostname" : "ssh.yourcompany.com",
		"port" : int64(22),
		"enabled" : true,
		"nested" : map[string]interface{}{
			"hello":    "hi",
			"fromage?": "that's cheese",
			"count":    1000.32,
			"how" : map[string]interface{}{
				"nested" : map[string]interface{}{
					"can" : "it be?",
					"beep" : int64(2),
					"bop" : false,
				},
			},
		},
	})
	ctx.requireCreate(config)

	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		ids, _, err := ctx.stores.Config.QueryIds(tx, `data.enabled and data.nested.hello = "hi"`)
		ctx.NoError(err)
		ctx.Equal(1, len(ids))
		ctx.Equal(config.Id, ids[0])

		ids, _, err = ctx.stores.Config.QueryIds(tx, `data.enabled and data.nested.how.nested.beep = 2`)
		ctx.NoError(err)
		ctx.Equal(1, len(ids))
		ctx.Equal(config.Id, ids[0])

		ids, _, err = ctx.stores.Config.QueryIds(tx, `data.enabled and data.nested.how.nested.beep = 3`)
		ctx.NoError(err)
		ctx.Equal(0, len(ids))
		return nil
	})
	ctx.NoError(err)

	ctx.requireDelete(config)
}
