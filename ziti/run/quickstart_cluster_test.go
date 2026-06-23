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
	"bytes"
	"os"
	"strings"
	"testing"
)

// argValue returns the element following the first occurrence of flag, or "" if
// flag is absent or has no following element. childArgs emits flags and values
// as separate argv elements (e.g. "--ctrl-port", "1280").
func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func hasArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func assertArg(t *testing.T, args []string, flag, want string) {
	t.Helper()
	if got := argValue(args, flag); got != want {
		t.Errorf("%s = %q, want %q (args: %v)", flag, got, want, args)
	}
}

func TestClusterChildArgs_InitNode(t *testing.T) {
	home := t.TempDir()
	o := &QuickstartClusterOpts{
		Home: home, Username: "admin", Password: "secret",
		CtrlPort: 1280, RouterPort: 3022, TrustDomain: "quickstart", Size: 3,
	}
	args := o.childArgs(0, "ctrl.example")

	if len(args) < 2 || args[0] != "run" || args[1] != "quickstart" {
		t.Fatalf("expected 'run quickstart' prefix, got %v", args)
	}
	if hasArg(args, "join") {
		t.Errorf("node 0 must not be a join: %v", args)
	}
	if hasArg(args, "--cluster-member") {
		t.Errorf("node 0 must have no --cluster-member: %v", args)
	}
	assertArg(t, args, "--instance-id", "instance-1")
	assertArg(t, args, "--ctrl-port", "1280")
	assertArg(t, args, "--router-port", "3022")
	assertArg(t, args, "--trust-domain", "quickstart")
	assertArg(t, args, "--username", "admin")
	assertArg(t, args, "--password", "secret")
	assertArg(t, args, "--home", home)
}

func TestClusterChildArgs_JoinNodePortsAndMember(t *testing.T) {
	o := &QuickstartClusterOpts{
		Home: t.TempDir(), Username: "admin", Password: "admin",
		CtrlPort: 1280, RouterPort: 3022, TrustDomain: "qs", Size: 3,
	}
	// third node (index 2)
	args := o.childArgs(2, "ctrl.example")

	if !hasArg(args, "join") {
		t.Errorf("node 2 must be a join: %v", args)
	}
	assertArg(t, args, "--instance-id", "instance-3")
	// ports are base + index
	assertArg(t, args, "--ctrl-port", "1282")
	assertArg(t, args, "--router-port", "3024")
	// every join targets node 0's controller (the BASE ctrl port), not its own
	assertArg(t, args, "--cluster-member", "tls:ctrl.example:1280")
}

func TestClusterChildArgs_OptionalFlags(t *testing.T) {
	with := &QuickstartClusterOpts{
		Home: t.TempDir(), CtrlPort: 1280, RouterPort: 3022,
		ControllerAddress: "cadr", RouterAddress: "radr", verbose: true,
	}
	a := with.childArgs(0, "cadr")
	assertArg(t, a, "--ctrl-address", "cadr")
	assertArg(t, a, "--router-address", "radr")
	if !hasArg(a, "--verbose") {
		t.Errorf("expected --verbose when set: %v", a)
	}

	without := &QuickstartClusterOpts{Home: t.TempDir(), CtrlPort: 1280, RouterPort: 3022}
	b := without.childArgs(0, "x")
	if hasArg(b, "--ctrl-address") || hasArg(b, "--router-address") {
		t.Errorf("address flags must be omitted when unset: %v", b)
	}
	if hasArg(b, "--verbose") {
		t.Errorf("--verbose must be omitted when unset: %v", b)
	}
}

func TestPrefixWriter_PrefixesAndBuffersPartialLines(t *testing.T) {
	var buf bytes.Buffer
	w := newPrefixWriter(&buf, "[n1] ")

	// a partial line should be held until its newline arrives
	_, _ = w.Write([]byte("hello\nwor"))
	if got := buf.String(); got != "[n1] hello\n" {
		t.Fatalf("after partial write got %q", got)
	}
	_, _ = w.Write([]byte("ld\n"))
	if got, want := buf.String(), "[n1] hello\n[n1] world\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReadyWriter_FiresOnceOnSentinel(t *testing.T) {
	var buf bytes.Buffer
	ready := make(chan struct{})
	calls := 0
	w := newReadyWriter(&buf, "[n1] ", "READY", func() {
		calls++
		close(ready)
	})

	_, _ = w.Write([]byte("starting up\n"))
	select {
	case <-ready:
		t.Fatal("sentinel fired before its line was written")
	default:
	}

	// two matching lines in one write, onSentinel must fire exactly once
	_, _ = w.Write([]byte("now READY to serve\nstill READY\n"))
	select {
	case <-ready:
	default:
		t.Fatal("expected ready to be signaled after sentinel line")
	}

	// further matches must not re-invoke (a second close() would panic)
	_, _ = w.Write([]byte("READY yet again\n"))
	if calls != 1 {
		t.Errorf("onSentinel called %d times, want 1", calls)
	}
}

func TestResolveHome_TempCreatedAndMarkedForCleanup(t *testing.T) {
	o := &QuickstartClusterOpts{}
	if err := o.resolveHome(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if o.Home != "" {
			_ = os.RemoveAll(o.Home)
		}
	}()

	if o.Home == "" {
		t.Fatal("expected a temp home to be created")
	}
	if !o.cleanOnExit {
		t.Error("temp home must be marked cleanOnExit")
	}
	if _, err := os.Stat(o.Home); err != nil {
		t.Errorf("temp home should exist: %v", err)
	}
}

func TestResolveHome_ExplicitHomeNotCleaned(t *testing.T) {
	dir := t.TempDir()
	o := &QuickstartClusterOpts{Home: dir}
	if err := o.resolveHome(); err != nil {
		t.Fatal(err)
	}
	if o.cleanOnExit {
		t.Error("explicit --home must not be marked cleanOnExit")
	}
	if o.Home != dir {
		t.Errorf("explicit --home changed: got %q want %q", o.Home, dir)
	}
}

func TestResolveHome_TildeExpanded(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no user home dir available")
	}
	o := &QuickstartClusterOpts{Home: "~/some-sub"}
	if err := o.resolveHome(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(o.Home, "~") {
		t.Errorf("tilde not expanded: %q", o.Home)
	}
	if !strings.HasPrefix(o.Home, home) {
		t.Errorf("expanded home %q should start with %q", o.Home, home)
	}
	if o.cleanOnExit {
		t.Error("explicit ~ home must not be marked cleanOnExit")
	}
}
