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
	"testing"

	"github.com/stretchr/testify/require"
)

func loadManaged(t *testing.T, src map[interface{}]interface{}) *ManagedConfigOptions {
	t.Helper()
	opts := &ManagedConfigOptions{}
	err := LoadManagedConfigFromMap(src, opts)
	require.NoError(t, err)
	return opts
}

func Test_ManagedConfig_AbsentSection(t *testing.T) {
	req := require.New(t)
	opts := loadManaged(t, map[interface{}]interface{}{})
	req.Empty(opts.Allow)
	req.False(opts.IsAllowed("router.link.v1"))
	req.False(opts.IsAllowed("anything"))
}

func Test_ManagedConfig_NilSection(t *testing.T) {
	req := require.New(t)
	opts := loadManaged(t, map[interface{}]interface{}{"managedConfig": nil})
	req.Empty(opts.Allow)
	req.False(opts.IsAllowed("router.link.v1"))
}

func Test_ManagedConfig_EmptyAllow(t *testing.T) {
	req := require.New(t)
	opts := loadManaged(t, map[interface{}]interface{}{
		"managedConfig": map[interface{}]interface{}{
			"allow": []interface{}{},
		},
	})
	req.Empty(opts.Allow)
	req.False(opts.IsAllowed("router.link.v1"))
}

func Test_ManagedConfig_AllKeyword(t *testing.T) {
	req := require.New(t)
	opts := loadManaged(t, map[interface{}]interface{}{
		"managedConfig": map[interface{}]interface{}{
			"allow": []interface{}{"all"},
		},
	})
	req.True(opts.IsAllowed("router.link.v1"))
	req.True(opts.IsAllowed("router.xgress.tunnel.v1"))
	req.True(opts.IsAllowed("anything-at-all"))
}

func Test_ManagedConfig_ExactMatch(t *testing.T) {
	req := require.New(t)
	opts := loadManaged(t, map[interface{}]interface{}{
		"managedConfig": map[interface{}]interface{}{
			"allow": []interface{}{"router.link.v1"},
		},
	})
	req.True(opts.IsAllowed("router.link.v1"))
	req.False(opts.IsAllowed("router.link.v2"))
	req.False(opts.IsAllowed("router.forwarder"))
}

func Test_ManagedConfig_FamilyPrefix(t *testing.T) {
	req := require.New(t)
	opts := loadManaged(t, map[interface{}]interface{}{
		"managedConfig": map[interface{}]interface{}{
			"allow": []interface{}{"router.link", "router.xgress.proxy"},
		},
	})
	req.True(opts.IsAllowed("router.link.v1"))
	req.True(opts.IsAllowed("router.link.v2"))
	req.True(opts.IsAllowed("router.link.v42"))
	req.True(opts.IsAllowed("router.xgress.proxy.v1"))
	req.False(opts.IsAllowed("router.xgress.tunnel.v1"))
	req.False(opts.IsAllowed("router.linker.v1"), "trailing dot should prevent false-prefix match")
	req.True(opts.IsAllowed("router.link"), "an exact match against the bare family entry is also allowed")
}

func Test_ManagedConfig_VersionSuffixIsStrict(t *testing.T) {
	req := require.New(t)
	opts := loadManaged(t, map[interface{}]interface{}{
		"managedConfig": map[interface{}]interface{}{
			"allow": []interface{}{"router.link"},
		},
	})
	// only entry + ".v" + digits matches; other descendants do not
	req.False(opts.IsAllowed("router.link.subpath"), "non-version descendants must not match")
	req.False(opts.IsAllowed("router.link.v1.x"), "version suffix must be the trailing segment")
	req.False(opts.IsAllowed("router.link.v"), "empty version number must not match")
	req.False(opts.IsAllowed("router.link.va"), "version suffix must be digits")
	req.False(opts.IsAllowed("router.link.v1a"), "trailing non-digit must not match")
	req.False(opts.IsAllowed("router.link.V1"), "version marker is case-sensitive")
}

func Test_ManagedConfig_MixedAllAndExplicit(t *testing.T) {
	req := require.New(t)
	// "all" anywhere short-circuits; the rest of the list is irrelevant.
	opts := loadManaged(t, map[interface{}]interface{}{
		"managedConfig": map[interface{}]interface{}{
			"allow": []interface{}{"router.link", "all"},
		},
	})
	req.True(opts.IsAllowed("router.xgress.tunnel.v1"))
	req.True(opts.IsAllowed("anything"))
}

func Test_ManagedConfig_InvalidShape(t *testing.T) {
	req := require.New(t)

	// managedConfig must be a map
	err := LoadManagedConfigFromMap(map[interface{}]interface{}{
		"managedConfig": "not a map",
	}, &ManagedConfigOptions{})
	req.Error(err)
	req.Contains(err.Error(), "[managedConfig]")

	// allow must be a list
	err = LoadManagedConfigFromMap(map[interface{}]interface{}{
		"managedConfig": map[interface{}]interface{}{
			"allow": "not a list",
		},
	}, &ManagedConfigOptions{})
	req.Error(err)
	req.Contains(err.Error(), "[managedConfig/allow]")
}

func Test_ManagedConfig_NilReceiver_IsAllowed(t *testing.T) {
	req := require.New(t)
	var opts *ManagedConfigOptions
	req.False(opts.IsAllowed("anything"), "nil receiver should be the disabled state")
}
