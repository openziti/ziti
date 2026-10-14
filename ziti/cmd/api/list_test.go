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

package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIdOrName(t *testing.T) {
	tests := []struct {
		val    string
		id     string
		filter string
	}{
		{val: "id:abc123", id: "abc123"},
		{val: "name:my-service", filter: `name="my-service"`},
		{val: "my-service", filter: `id="my-service" or name="my-service"`},
		{val: "identity-svc", filter: `id="identity-svc" or name="identity-svc"`},
		{val: "idp", filter: `id="idp" or name="idp"`},
		{val: "namespace-router", filter: `id="namespace-router" or name="namespace-router"`},
		{val: "name:id:odd", filter: `name="id:odd"`},
	}
	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			id, filter := parseIdOrName(tt.val)
			require.Equal(t, tt.id, id)
			require.Equal(t, tt.filter, filter)
		})
	}
}
