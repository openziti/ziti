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

package model

import (
	"testing"

	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/fields"
	"go.etcd.io/bbolt"
)

func TestConfigTypeUpdateKeepsSystemVersion(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	tests := []struct {
		name       string
		configType string
		checker    fields.UpdatedFields
	}{
		{name: "replace", configType: "host.v1", checker: nil},
		{name: "patch", configType: "host.v2", checker: fields.UpdatedFieldsMap{db.FieldConfigTypeSchema: struct{}{}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx.NextTest(t)

			configType, err := ctx.managers.ConfigType.ReadByName(tc.configType)
			ctx.NoError(err)
			configType.Schema = map[string]interface{}{"type": "object", "description": tc.name}
			ctx.NoError(ctx.managers.ConfigType.Update(configType, tc.checker, change.New()))

			ctx.NoError(ctx.GetDb().View(func(tx *bbolt.Tx) error {
				stored, err := ctx.GetStores().ConfigType.LoadOneByName(tx, tc.configType)
				ctx.NoError(err)
				ctx.Equal(tc.name, stored.Schema["description"])
				ctx.Positive(stored.SystemVersion)
				return nil
			}))
		})
	}
}
