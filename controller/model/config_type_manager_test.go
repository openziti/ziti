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

	"github.com/openziti/storage/boltz"
	"github.com/openziti/storage/boltztest"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/fields"
)

// A config type's SystemVersion is hidden from the API, so no update request carries it. If an API
// update wrote it anyway, the next startup would see a lower version and revert the update's schema.
func TestConfigTypeUpdateKeepsSystemVersion(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	tests := []struct {
		name       string
		configType string
		checker    fields.UpdatedFields
	}{
		{name: "replace keeps the stored version", configType: "host.v1", checker: nil},
		{
			name:       "patch keeps the stored version",
			configType: "host.v2",
			checker:    fields.UpdatedFieldsMap{db.FieldConfigTypeSchema: struct{}{}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx.NextTest(t)
			configType, err := ctx.managers.ConfigType.ReadByName(tc.configType)
			ctx.NoError(err)
			before := &db.ConfigType{BaseExtEntity: boltz.BaseExtEntity{Id: configType.Id}}
			ctx.NoError(boltztest.Reload(ctx, before))
			ctx.Positive(before.SystemVersion, "system config types are synced, and so versioned, at startup")
			configType.Schema = map[string]interface{}{"type": "object", "description": tc.name}

			err = ctx.managers.ConfigType.Update(configType, tc.checker, change.New())

			ctx.NoError(err)
			after := &db.ConfigType{BaseExtEntity: boltz.BaseExtEntity{Id: configType.Id}}
			ctx.NoError(boltztest.Reload(ctx, after))
			ctx.Equal(tc.name, after.Schema["description"], "the update should write the schema")
			ctx.Equal(before.SystemVersion, after.SystemVersion, "the update should keep the stored system version")
		})
	}
}
