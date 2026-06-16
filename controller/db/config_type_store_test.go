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
	"github.com/openziti/ziti/v2/controller/storage/boltztest"
	"github.com/xeipuuv/gojsonschema"
	"go.etcd.io/bbolt"
)

func Test_ConfigTypeStore(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	t.Run("test config type CRUD", ctx.testConfigTypeCrud)
	t.Run("test config type target immutability", ctx.testConfigTypeTargetImmutability)
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

func Test_RouterLinkV1Builtin(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()
	ctx.Init()

	var stored *ConfigType
	err := ctx.GetDb().View(func(tx *bbolt.Tx) error {
		var loadErr error
		stored, loadErr = ctx.stores.ConfigType.LoadOneByName(tx, RouterLinkV1TypeId)
		return loadErr
	})
	ctx.NoError(err)
	ctx.NotNil(stored, "router.link.v1 must be registered as a built-in")
	ctx.Equal(RouterLinkV1TypeId, stored.Id)
	ctx.Equal(RouterLinkV1TypeId, stored.Name)
	ctx.Equal(ConfigTypeTargetRouter, stored.Target)

	// Compile the schema. migration_initialize.go writes config types directly via the
	// store, bypassing the model-level GetCompiledSchema check, so a malformed $ref or
	// oneOf would only blow up at first config-create. Compile here to catch it eagerly.
	schema, err := gojsonschema.NewSchemaLoader().Compile(gojsonschema.NewGoLoader(stored.Schema))
	ctx.NoError(err)
	ctx.NotNil(schema)

	validate := func(payload map[string]interface{}) *gojsonschema.Result {
		result, err := schema.Validate(gojsonschema.NewGoLoader(payload))
		ctx.NoError(err)
		return result
	}

	t.Run("valid payload", func(t *testing.T) {
		ctx.NextTest(t)
		result := validate(map[string]interface{}{
			"listeners": []interface{}{
				map[string]interface{}{
					"binding":   "transport",
					"bind":      "tls:0.0.0.0:6262",
					"advertise": "tls:public.example:6262",
					"groups":    []interface{}{"default"},
					"options": map[string]interface{}{
						"outQueueSize":   16,
						"connectTimeout": "5s",
					},
				},
			},
			"dialers": []interface{}{
				map[string]interface{}{
					"binding":              "transport",
					"maxDefaultConnections": 4,
					"healthyDialBackoff": map[string]interface{}{
						"retryBackoffFactor": 2,
						"minRetryInterval":   "1s",
						"maxRetryInterval":   "1m",
					},
				},
			},
			"heartbeats": map[string]interface{}{
				"sendInterval": "10s",
			},
			"payloadSenderQueueSize": 256,
			"ackSenderQueueSize":     128,
			"gcMode":                 "orphaned",
		})
		ctx.True(result.Valid(), "expected payload to validate, errors: %v", result.Errors())
	})

	t.Run("binding optional on listener and dialer", func(t *testing.T) {
		ctx.NextTest(t)
		result := validate(map[string]interface{}{
			"listeners": []interface{}{
				map[string]interface{}{"bind": "tls:0.0.0.0:6262"},
			},
			"dialers": []interface{}{
				map[string]interface{}{},
			},
		})
		ctx.True(result.Valid(), "expected binding-less payload to validate, errors: %v", result.Errors())
	})

	durationPayload := func(d interface{}) map[string]interface{} {
		return map[string]interface{}{
			"listeners": []interface{}{
				map[string]interface{}{
					"bind":    "tls:0.0.0.0:6262",
					"options": map[string]interface{}{"connectTimeout": d},
				},
			},
		}
	}

	t.Run("duration formats", func(t *testing.T) {
		ctx.NextTest(t)
		// these mirror what time.ParseDuration accepts
		for _, d := range []string{"1h", "1h30m", "1.5h", "300ms", "500us", "10s", "1m", "200ns"} {
			result := validate(durationPayload(d))
			ctx.True(result.Valid(), "expected duration %q to validate, errors: %v", d, result.Errors())
		}
		for _, d := range []string{"", "1", "30", "1h30", "1 m", "1x", "h", "1hh"} {
			result := validate(durationPayload(d))
			ctx.False(result.Valid(), "expected duration %q to be rejected", d)
		}
	})

	rejects := []struct {
		name    string
		payload map[string]interface{}
	}{
		{
			name:    "extra top-level key",
			payload: map[string]interface{}{"bogus": true},
		},
		{
			name: "listener missing bind",
			payload: map[string]interface{}{
				"listeners": []interface{}{map[string]interface{}{"binding": "transport"}},
			},
		},
		{
			name: "maxDefaultConnections out of range",
			payload: map[string]interface{}{
				"dialers": []interface{}{
					map[string]interface{}{"maxDefaultConnections": 0},
				},
			},
		},
		{
			name: "retryBackoffFactor below minimum",
			payload: map[string]interface{}{
				"dialers": []interface{}{
					map[string]interface{}{
						"healthyDialBackoff": map[string]interface{}{"retryBackoffFactor": 0.5},
					},
				},
			},
		},
		{
			name: "groups wrong type",
			payload: map[string]interface{}{
				"listeners": []interface{}{
					map[string]interface{}{"bind": "tls:0.0.0.0:6262", "groups": 42},
				},
			},
		},
		{
			name: "channelOptions.maxQueuedConnects below minimum",
			payload: map[string]interface{}{
				"listeners": []interface{}{
					map[string]interface{}{
						"bind":    "tls:0.0.0.0:6262",
						"options": map[string]interface{}{"maxQueuedConnects": 0},
					},
				},
			},
		},
		{
			name:    "gcMode not in enum",
			payload: map[string]interface{}{"gcMode": "aggressive"},
		},
	}
	for _, tc := range rejects {
		t.Run("rejects: "+tc.name, func(t *testing.T) {
			ctx.NextTest(t)
			result := validate(tc.payload)
			ctx.False(result.Valid(), "expected payload to be rejected")
		})
	}
}

func (ctx *TestContext) testConfigTypeTargetImmutability(*testing.T) {
	ctx.CleanupAll()

	configType := newConfigType(eid.New())
	configType.Target = ConfigTypeTargetService
	boltztest.RequireCreate(ctx, configType)

	// no-op rewrite of the same target value should succeed
	boltztest.RequireUpdate(ctx, configType)

	// changing the target should be rejected
	configType.Target = ConfigTypeTargetRouter
	err := boltztest.Update(ctx, configType)
	ctx.Error(err)
	ctx.Contains(err.Error(), "target is immutable")

	// the stored target should remain unchanged
	err = ctx.GetDb().View(func(tx *bbolt.Tx) error {
		stored, loadErr := ctx.stores.ConfigType.LoadById(tx, configType.Id)
		ctx.NoError(loadErr)
		ctx.Equal(ConfigTypeTargetService, stored.Target)
		return nil
	})
	ctx.NoError(err)

	// reset and clean up
	configType.Target = ConfigTypeTargetService
	boltztest.RequireDelete(ctx, configType)
}
