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

package database

import (
	"fmt"
	"testing"

	"github.com/openziti/ziti/v2/controller/db"
	"github.com/stretchr/testify/require"
)

// The settable range is the numbered versions a controller with named migrations converts from without
// re-running a numbered migration.
func TestCheckSettableDbVersion(t *testing.T) {
	tests := []struct {
		version int
		valid   bool
	}{
		{version: 0, valid: false},
		{version: db.LegacyDbVersion - 1, valid: false},
		{version: db.LegacyDbVersion, valid: true},
		{version: db.MaxLegacyDbVersion, valid: true},
		{version: db.MaxLegacyDbVersion + 1, valid: false},
		{version: db.NamedMigrationsDbVersion, valid: false},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("version %d", tc.version), func(t *testing.T) {
			err := checkSettableDbVersion(tc.version)

			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "outside the settable range")
			}
		})
	}
}
