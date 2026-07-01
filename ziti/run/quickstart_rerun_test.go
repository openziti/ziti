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

package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitHostPortU16(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
		wantPort uint16
		wantOK   bool
	}{
		{"sg4:20000", "sg4", 20000, true},
		{"localhost:1280", "localhost", 1280, true},
		{"[::1]:8443", "::1", 8443, true},
		{"host", "", 0, false},              // missing port
		{"host:0", "", 0, false},            // zero port
		{"host:70000", "", 0, false},        // out of range
		{"host:-1", "", 0, false},           // negative
		{"tls:host:20000", "", 0, false},    // scheme-prefixed, not a bare host:port
		{"", "", 0, false},                  // empty
		{"  sg4:20000  ", "sg4", 20000, true}, // trimmed
	}
	for _, c := range cases {
		host, port, ok := splitHostPortU16(c.in)
		if ok != c.wantOK || host != c.wantHost || port != c.wantPort {
			t.Errorf("splitHostPortU16(%q) = (%q, %d, %v), want (%q, %d, %v)",
				c.in, host, port, ok, c.wantHost, c.wantPort, c.wantOK)
		}
	}
}

func TestReadAdvertisedFromCtrlConfig(t *testing.T) {
	dir := t.TempDir()

	// Mirrors the web bindPoint section of config_templates/controller.yml.
	good := filepath.Join(dir, "ctrl.yaml")
	writeFile(t, good, `
web:
  - name: client-management
    bindPoints:
      - interface: 0.0.0.0:20000
        address: sg4:20000
`)
	host, port, ok := readAdvertisedFromCtrlConfig(good)
	if !ok || host != "sg4" || port != 20000 {
		t.Fatalf("ctrl config = (%q, %d, %v), want (sg4, 20000, true)", host, port, ok)
	}

	// No web section: fail safe (caller keeps flag/default).
	noWeb := filepath.Join(dir, "noweb.yaml")
	writeFile(t, noWeb, "ctrl:\n  listener: tls:0.0.0.0:6262\n")
	if _, _, ok := readAdvertisedFromCtrlConfig(noWeb); ok {
		t.Error("expected ok=false for a config with no web listeners")
	}

	// Missing file: fail safe.
	if _, _, ok := readAdvertisedFromCtrlConfig(filepath.Join(dir, "nope.yaml")); ok {
		t.Error("expected ok=false for a missing file")
	}
}

func TestReadAdvertisedFromRouterConfig(t *testing.T) {
	dir := t.TempDir()

	// Mirrors the edge listener section of config_templates/router.yml, including the transport
	// link listener whose advertise is tls-prefixed and must NOT be picked.
	good := filepath.Join(dir, "router.yaml")
	writeFile(t, good, `
link:
  listeners:
    - binding: transport
      bind: tls:0.0.0.0:20001
      advertise: tls:sg4:20001
listeners:
  - binding: edge
    address: tls:0.0.0.0:20001
    options:
      advertise: sg4:20001
  - binding: tunnel
    options:
      mode: host
`)
	host, port, ok := readAdvertisedFromRouterConfig(good)
	if !ok || host != "sg4" || port != 20001 {
		t.Fatalf("router config = (%q, %d, %v), want (sg4, 20001, true)", host, port, ok)
	}

	// No edge listener: fail safe.
	noEdge := filepath.Join(dir, "noedge.yaml")
	writeFile(t, noEdge, "listeners:\n  - binding: tunnel\n    options:\n      mode: host\n")
	if _, _, ok := readAdvertisedFromRouterConfig(noEdge); ok {
		t.Error("expected ok=false for a router config with no edge listener")
	}
}

func TestNoteIgnoredSetupFlags(t *testing.T) {
	// username/password are intentionally NOT reported as ignored (a crash-resume login uses them).
	o := &QuickstartOpts{changedFlags: map[string]bool{
		"ctrl-port": true,
		"username":  true,
		"password":  true,
		"home":      true, // takes effect on a re-run, so not reported
	}}
	o.noteIgnoredSetupFlags()
	if len(o.ignoredSetupFlags) != 1 || o.ignoredSetupFlags[0] != "--ctrl-port" {
		t.Fatalf("ignoredSetupFlags = %v, want [--ctrl-port]", o.ignoredSetupFlags)
	}

	// No setup-only flag changed: empty.
	o2 := &QuickstartOpts{changedFlags: map[string]bool{"zac": true, "home": true}}
	o2.noteIgnoredSetupFlags()
	if len(o2.ignoredSetupFlags) != 0 {
		t.Fatalf("ignoredSetupFlags = %v, want empty", o2.ignoredSetupFlags)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
