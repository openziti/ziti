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

	"github.com/stretchr/testify/require"
)

func TestCheckControllerLine(t *testing.T) {
	tests := []struct {
		name         string
		ctrlVersion  string
		buildVersion string
		err          string
	}{
		{name: "same patch", ctrlVersion: "v2.0.8", buildVersion: "v2.0.8"},
		{name: "older patch", ctrlVersion: "v2.0.8", buildVersion: "v2.0.9"},
		{name: "without the v", ctrlVersion: "2.0.8", buildVersion: "v2.0.9"},
		{name: "prerelease of the line", ctrlVersion: "v2.1.0-pre3", buildVersion: "v2.1.0"},
		{name: "unstamped build", ctrlVersion: "v1.5.4", buildVersion: unstampedVersion},
		{
			name:         "older line",
			ctrlVersion:  "v2.0.8",
			buildVersion: "v2.1.0",
			err:          "ctrl1 on v2.0.8 is not on this build's 2.1 line (v2.1.0), whose tests and config target 2.1 controllers; run it from release-v2.0.x",
		},
		{
			name:         "newer line",
			ctrlVersion:  "v2.0.8",
			buildVersion: "v1.6.23",
			err:          "ctrl1 on v2.0.8 is not on this build's 1.6 line (v1.6.23), whose tests and config target 1.6 controllers; run it from release-v2.0.x",
		},
		{name: "unparsable", ctrlVersion: "latest", buildVersion: "v2.1.0", err: `ctrl1 version "latest"`},
		{name: "no minor", ctrlVersion: "v2", buildVersion: "v2.1.0", err: `ctrl1 version "v2"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkControllerLine("ctrl1", test.ctrlVersion, test.buildVersion)
			if test.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.err)
			}
		})
	}
}
