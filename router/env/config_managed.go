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
	"fmt"
	"strings"
)

// ManagedConfigAllowAll is the special allow-list entry that accepts every
// config type the controller sends.
const ManagedConfigAllowAll = "all"

// ManagedConfigOptions holds the local router's allow-list for
// controller-managed config types. See doc/design/ctrl-managed-router-config.md
// "Local Config Type Allow-list" for the contract.
type ManagedConfigOptions struct {
	// Allow is the operator's allow-list, exactly as written in YAML.
	//   - empty/nil  -> controller-managed config is disabled
	//   - ["all"]    -> every config type is accepted
	//   - otherwise  -> only matching entries are accepted (see IsAllowed)
	Allow []string
}

// LoadManagedConfigFromMap parses the optional `managedConfig` section out of
// a router YAML config map and writes the result into options. Resets
// options.Allow first, so the function is safe to call repeatedly on the same
// struct. An absent section leaves options.Allow nil (controller-managed
// config disabled, which is the safe default).
func LoadManagedConfigFromMap(cfgmap map[interface{}]interface{}, options *ManagedConfigOptions) error {
	options.Allow = nil
	value, found := cfgmap["managedConfig"]
	if !found || value == nil {
		return nil
	}
	submap, ok := value.(map[interface{}]interface{})
	if !ok {
		return fmt.Errorf("[managedConfig] must be a map, got %T", value)
	}
	allowVal, found := submap["allow"]
	if !found || allowVal == nil {
		return nil
	}
	allowList, ok := allowVal.([]interface{})
	if !ok {
		return fmt.Errorf("[managedConfig/allow] must be a list, got %T", allowVal)
	}
	for _, item := range allowList {
		options.Allow = append(options.Allow, fmt.Sprint(item))
	}
	return nil
}

// IsAllowed reports whether the given config type name should be accepted from
// the controller. Returns false when the allow-list is empty (managed config
// disabled). Returns true if the list contains "all". Otherwise returns true
// when the type matches an entry exactly OR is a versioned child of an entry,
// i.e. the type equals entry + ".v" + <digits>. No other descendants match.
//
// Examples (with Allow = ["router.link", "router.xgress.proxy"]):
//
//	"router.link"             -> true (exact)
//	"router.link.v1"          -> true (versioned child)
//	"router.link.v2"          -> true (versioned child)
//	"router.link.v1.x"        -> false (suffix after .v must be digits only)
//	"router.link.subpath"     -> false (not a versioned child)
//	"router.xgress.proxy.v1"  -> true
//	"router.xgress.tunnel.v1" -> false (not under any allowed entry)
//	"router.linker.v1"        -> false (not a child; the trailing dot matters)
func (m *ManagedConfigOptions) IsAllowed(configType string) bool {
	if m == nil || len(m.Allow) == 0 {
		return false
	}
	for _, entry := range m.Allow {
		if entry == ManagedConfigAllowAll {
			return true
		}
		if entry == configType {
			return true
		}
		if isVersionedChild(configType, entry) {
			return true
		}
	}
	return false
}

// isVersionedChild reports whether configType is of the form entry + ".v" + N,
// where N is one or more decimal digits.
func isVersionedChild(configType, entry string) bool {
	suffix, ok := strings.CutPrefix(configType, entry+".v")
	if !ok || suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
