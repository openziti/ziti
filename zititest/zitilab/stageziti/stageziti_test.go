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

package stageziti

import "testing"

// TestZitiReleaseVersionRe pins the routing decision in StageZiti: only
// release-tag-shaped strings should be passed to getziti.InstallZiti; everything
// else (hashes, branches) gets built from source.
func TestZitiReleaseVersionRe(t *testing.T) {
	cases := []struct {
		version string
		release bool
	}{
		// Release tags (download path)
		{"v2.0.0", true},
		{"v2.0.0-pre11", true},
		{"v0.31.0", true},
		{"v1.1.15", true},
		{"v10.20.30", true},
		{"v2.0.0-rc.1", true},

		// Non-release (build-from-git path)
		{"acd83a9f267221", false},                   // commit hash
		{"acd83a9f267221abcdef1234567890abcdef1234", false}, // full SHA
		{"stall-testing", false},                    // branch
		{"main", false},                             // branch
		{"HEAD", false},                             // branch ref
		{"2.0.0", false},                            // missing v prefix
		{"v2", false},                               // not full semver
		{"v2.0", false},                             // not full semver
		{"vacd83a9", false},                         // v-prefixed hash, still not semver
	}
	for _, tc := range cases {
		got := zitiReleaseVersionRe.MatchString(tc.version)
		if got != tc.release {
			t.Errorf("zitiReleaseVersionRe.MatchString(%q) = %v, want %v",
				tc.version, got, tc.release)
		}
	}
}
