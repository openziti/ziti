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

	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

// Test_L2HostV1Schema verifies that l2.host.v1 resolves the definitions its listenOptions refer
// to, and does not accept health checks, which don't apply to l2 services.
func Test_L2HostV1Schema(t *testing.T) {
	req := require.New(t)
	schema, err := gojsonschema.NewSchemaLoader().Compile(gojsonschema.NewGoLoader(l2HostV1ConfigType.Schema))
	req.NoError(err)

	validate := func(payload map[string]interface{}) bool {
		result, err := schema.Validate(gojsonschema.NewGoLoader(payload))
		req.NoError(err)
		return result.Valid()
	}

	req.True(validate(map[string]interface{}{
		"bridgeIfs": []interface{}{"eth1"},
		"listenOptions": map[string]interface{}{
			"connectTimeout":        "5s",
			"connectTimeoutSeconds": 5,
		},
	}))
	req.False(validate(map[string]interface{}{
		"listenOptions": map[string]interface{}{"connectTimeout": "five seconds"},
	}), "connectTimeout should be validated as a duration")
	req.False(validate(map[string]interface{}{
		"portChecks": []interface{}{map[string]interface{}{"address": "localhost:80"}},
	}), "health checks should not be accepted")
}
