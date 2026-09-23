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

const minimalRouterConfig = `v: 3
identity:
  cert: c.pem
  server_cert: s.pem
  key: k.pem
  ca: ca.pem
ctrl:
  endpoint: tls:127.0.0.1:6262
csr:
  country: US
  province: NC
  locality: Charlotte
  organization: NetFoundry
  organizationalUnit: Ops
  sans:
    dns: [localhost]
`

func loadRouterConfig(t *testing.T, yaml string) *Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "router.yml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0600))
	cfg, err := LoadConfigWithOptions(path, false)
	require.NoError(t, err)
	return cfg
}

func Test_LoadConfig_RecordsWhetherLinkSectionIsPresent(t *testing.T) {
	// Every link field is filled with a default whether or not the section was
	// written, so Configured is the only way to tell them apart.
	t.Run("absent", func(t *testing.T) {
		req := require.New(t)
		cfg := loadRouterConfig(t, minimalRouterConfig)
		req.False(cfg.Link.Configured)
		req.NotZero(cfg.Link.Heartbeats.SendInterval, "defaults are filled in regardless")
	})

	t.Run("present with only heartbeats", func(t *testing.T) {
		req := require.New(t)
		cfg := loadRouterConfig(t, minimalRouterConfig+"link:\n  heartbeats:\n    sendInterval: 5s\n")
		req.True(cfg.Link.Configured)
		req.Empty(cfg.Link.Listeners)
		req.Empty(cfg.Link.Dialers)
	})
}
