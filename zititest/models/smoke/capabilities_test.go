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

func TestCapabilitySupportedByWithBackport(t *testing.T) {
	capability := Capability{Name: "thing", MinVersion: "v2.0.6", Backports: []string{"v1.6.21"}, Description: "does the thing"}

	tests := []struct {
		name    string
		version string
		support Support
	}{
		{name: "line without a backport", version: "v1.5.4", support: Support{Reason: "router-west on v1.5.4 lacks thing, from v2.0.6 (backported to v1.6.21): does the thing"}},
		{name: "backport line, before the backport", version: "v1.6.20", support: Support{Reason: "router-west on v1.6.20 lacks thing, from v1.6.21: does the thing"}},
		{name: "the backport", version: "v1.6.21", support: Support{Supported: true}},
		{name: "after the backport", version: "v1.6.22", support: Support{Supported: true}},
		{name: "minimum line, before the minimum", version: "v2.0.5", support: Support{Reason: "router-west on v2.0.5 lacks thing, from v2.0.6 (backported to v1.6.21): does the thing"}},
		{name: "the minimum", version: "v2.0.6", support: Support{Supported: true}},
		{name: "newer line", version: "v2.1.0-pre2", support: Support{Supported: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.support, capability.supportedBy("router-west", test.version, "", false))
		})
	}
}

func TestCapabilityMinVersionsParse(t *testing.T) {
	for _, capability := range Capabilities {
		_, err := versions.ParseSemVer(capability.MinVersion)
		require.NoError(t, err, capability.Name)

		lines := map[string]bool{}
		for _, backport := range capability.Backports {
			line, err := releaseLine(backport)
			require.NoError(t, err, capability.Name)
			require.False(t, lines[line], "%s has two backports on %s", capability.Name, line)
			lines[line] = true
			require.Negative(t, versions.MustParseSemVer(backport).CompareTo(versions.MustParseSemVer(capability.MinVersion)),
				"%s backport %s is not older than %s", capability.Name, backport, capability.MinVersion)
		}
	}
}

func TestSupportAnd(t *testing.T) {
	yes := Support{Supported: true}
	noClient := Support{Reason: "client router lacks it"}
	noHost := Support{Reason: "host router lacks it"}

	require.Equal(t, yes, yes.And(yes))
	require.Equal(t, noClient, noClient.And(yes))
	require.Equal(t, noHost, yes.And(noHost))
	require.Equal(t, Support{Reason: "client router lacks it; host router lacks it"}, noClient.And(noHost))
}
