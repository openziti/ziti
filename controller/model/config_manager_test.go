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

	"github.com/stretchr/testify/require"
)

func TestConfigManager_MarshallUnmarshallPreservesNumbers(t *testing.T) {
	req := require.New(t)
	manager := &ConfigManager{}
	config := &Config{
		Data: map[string]interface{}{
			"port": int64(443),
			"big":  int64(9007199254740993),
			"real": 1.5,
			"text": "443",
			"bool": true,
			"nested": map[string]interface{}{
				"port": int64(80),
			},
			"list": []interface{}{
				int64(443),
				map[string]interface{}{"big": int64(9007199254740993)},
				[]interface{}{int64(80), 1.5, "text", false},
			},
		},
	}

	encoded, err := manager.Marshall(config)
	req.NoError(err)

	decoded, err := manager.Unmarshall(encoded)
	req.NoError(err)
	req.Equal(config.Data, decoded.Data)
}
