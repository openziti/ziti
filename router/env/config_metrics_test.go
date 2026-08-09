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

package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_LoadConfig_hostMetrics(t *testing.T) {
	load := func(t *testing.T, metrics string) (*Config, error) {
		path := filepath.Join(t.TempDir(), "router.yml")
		require.NoError(t, os.WriteFile(path, []byte(minimalRouterConfig+"metrics:\n"+metrics), 0600))
		return LoadConfigWithOptions(path, false)
	}

	t.Run("absent leaves it off", func(t *testing.T) {
		cfg, err := load(t, "  reportInterval: 1m\n")
		require.NoError(t, err)
		require.False(t, cfg.Metrics.HostMetrics.Enabled)
	})

	t.Run("an empty stanza leaves it off", func(t *testing.T) {
		cfg, err := load(t, "  hostMetrics:\n")
		require.NoError(t, err)
		require.False(t, cfg.Metrics.HostMetrics.Enabled)
	})

	t.Run("enabled", func(t *testing.T) {
		cfg, err := load(t, "  hostMetrics:\n    enabled: true\n")
		require.NoError(t, err)
		require.True(t, cfg.Metrics.HostMetrics.Enabled)
	})

	t.Run("a stanza that is not a map is rejected", func(t *testing.T) {
		_, err := load(t, "  hostMetrics: true\n")
		require.ErrorContains(t, err, "metrics.hostMetrics")
	})

	t.Run("an enabled that is not a boolean is rejected", func(t *testing.T) {
		_, err := load(t, "  hostMetrics:\n    enabled: yes please\n")
		require.ErrorContains(t, err, "metrics.hostMetrics.enabled")
	})
}
