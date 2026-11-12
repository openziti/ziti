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

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_LoadNetworkConfig_hostMetrics(t *testing.T) {
	t.Run("absent leaves it off", func(t *testing.T) {
		options, err := LoadNetworkConfig(map[interface{}]interface{}{})
		require.NoError(t, err)
		require.False(t, options.HostMetrics.Enabled)
	})

	t.Run("an empty stanza leaves it off", func(t *testing.T) {
		options, err := LoadNetworkConfig(map[interface{}]interface{}{"hostMetrics": nil})
		require.NoError(t, err)
		require.False(t, options.HostMetrics.Enabled)
	})

	t.Run("enabled", func(t *testing.T) {
		options, err := LoadNetworkConfig(map[interface{}]interface{}{
			"hostMetrics": map[interface{}]interface{}{"enabled": true},
		})
		require.NoError(t, err)
		require.True(t, options.HostMetrics.Enabled)
	})

	t.Run("a stanza that is not a map is rejected", func(t *testing.T) {
		_, err := LoadNetworkConfig(map[interface{}]interface{}{"hostMetrics": true})
		require.ErrorContains(t, err, "hostMetrics")
	})

	t.Run("an enabled that is not a boolean is rejected", func(t *testing.T) {
		_, err := LoadNetworkConfig(map[interface{}]interface{}{
			"hostMetrics": map[interface{}]interface{}{"enabled": "yes"},
		})
		require.ErrorContains(t, err, "hostMetrics.enabled")
	})
}
