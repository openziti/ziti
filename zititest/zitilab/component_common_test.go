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

package zitilab

import "testing"

// TestCanonicalizeGoAppVersion pins the prefix-v behavior: bare semver gets
// the leading v added; hashes and branch names pass through unchanged so they
// can flow into the git-build path in stageziti.StageZiti.
func TestCanonicalizeGoAppVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Semver shapes — get the prefix
		{"2.0.0", "v2.0.0"},
		{"2.0.0-pre11", "v2.0.0-pre11"},
		{"0.31.0", "v0.31.0"},

		// Already prefixed — left alone
		{"v2.0.0", "v2.0.0"},
		{"v2.0.0-pre11", "v2.0.0-pre11"},

		// Sentinels
		{"", ""},
		{"latest", "latest"},

		// Commit hashes — pass through unchanged
		{"e68ee8d6f3ff7de241", "e68ee8d6f3ff7de241"},
		{"acd83a9f267221", "acd83a9f267221"},
		{"abc1234", "abc1234"},

		// Branch names — pass through unchanged
		{"main", "main"},
		{"stall-testing", "stall-testing"},
		{"HEAD", "HEAD"},
	}
	for _, tc := range cases {
		got := tc.in
		canonicalizeGoAppVersion(&got)
		if got != tc.want {
			t.Errorf("canonicalizeGoAppVersion(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}
