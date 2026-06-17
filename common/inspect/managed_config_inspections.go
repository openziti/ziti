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

package inspect

const (
	RouterConfigRegistryKey = "router-config-registry"
)

// RouterConfigRegistryState is the snapshot returned by the router config
// registry's Inspect method, suitable for JSON serialization (e.g.
// `ziti fabric inspect router-config-registry`).
type RouterConfigRegistryState struct {
	Sealed   bool                        `json:"sealed"`
	Closed   bool                        `json:"closed"`
	Handlers []RouterConfigHandlerDetail `json:"handlers"`
}

// RouterConfigHandlerDetail describes one registered handler: its base, the
// versions it understands, what data is currently known from each source, and
// what is currently applied.
type RouterConfigHandlerDetail struct {
	BaseType          string                      `json:"baseType"`
	SupportedVersions []int                       `json:"supportedVersions"`
	ControllerConfigs []RouterConfigVersionDetail `json:"controllerConfigs"`
	LocalConfig       *RouterConfigVersionDetail  `json:"localConfig,omitempty"`
	Applied           *RouterConfigAppliedDetail  `json:"applied,omitempty"`
}

// RouterConfigVersionDetail describes a single version-of-data the registry
// knows about. Data is the parsed JSON payload, inlined for readability when
// the inspect output is itself JSON-encoded.
type RouterConfigVersionDetail struct {
	Version int `json:"version"`
	Data    any `json:"data"`
}

// RouterConfigAppliedDetail describes the currently-applied config for a
// handler.
type RouterConfigAppliedDetail struct {
	Source  string `json:"source"`
	Version int    `json:"version"`
}
