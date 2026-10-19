/*
	(c) Copyright NetFoundry Inc.

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

package smoke

import (
	"testing"

	"github.com/openziti/foundation/v2/versions"
	"github.com/stretchr/testify/require"
)

func TestCapabilitySupportedBy(t *testing.T) {
	capability := Capability{Name: "thing", MinVersion: "v2.1.0-pre2", Description: "does the thing"}

	tests := []struct {
		name       string
		version    string
		override   string
		overridden bool
		support    Support
	}{
		{name: "local build", version: "", support: Support{Supported: true}},
		{name: "older release", version: "v1.6.21", support: Support{Reason: "router-west on v1.6.21 lacks thing, from v2.1.0-pre2: does the thing"}},
		{name: "older prerelease", version: "v2.1.0-pre1", support: Support{Reason: "router-west on v2.1.0-pre1 lacks thing, from v2.1.0-pre2: does the thing"}},
		{name: "the minimum", version: "v2.1.0-pre2", support: Support{Supported: true}},
		{name: "later prerelease", version: "v2.1.0-pre10", support: Support{Supported: true}},
		{name: "the release", version: "v2.1.0", support: Support{Supported: true}},
		{name: "newer release", version: "v2.2.3", support: Support{Supported: true}},
		{name: "unparsable", version: "latest", support: Support{Reason: "router-west on latest lacks thing, from v2.1.0-pre2: does the thing"}},
		{name: "forced on for an old router", version: "v1.1.15", override: "on", overridden: true, support: Support{Supported: true}},
		{name: "forced off for the local build", version: "", override: "off", overridden: true, support: Support{Reason: "router-west lacks thing (capability_thing=off)"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			support := capability.supportedBy("router-west", test.version, test.override, test.overridden)
			require.Equal(t, test.support, support)
		})
	}
}

func TestCapabilityMinVersionsParse(t *testing.T) {
	for _, capability := range Capabilities {
		_, err := versions.ParseSemVer(capability.MinVersion)
		require.NoError(t, err, capability.Name)
	}
}
