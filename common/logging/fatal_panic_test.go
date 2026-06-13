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

package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// subprocessEnv selects the child-mode behavior; an empty value means the
// process is the parent test driver. Held under one env var so both
// subprocess tests share the same dispatch.
const subprocessEnv = "ZITI_LOGGING_FATAL_PANIC_CHILD"

const (
	subprocessFatalMsg = "durable-fatal-marker"
	subprocessPanicMsg = "durable-panic-marker"
)

// TestFatalReachesStderrBeforeExit forks the test binary, has the child
// configure the production handler chain via BuildHandler over os.Stderr,
// then call logrus.Fatal. The parent asserts the child exited non-zero AND
// that the fatal marker appears in the child's stderr; together these prove
// the bridge's SyncEmit path flushed the record to the leaf handler before
// logrus called os.Exit.
func TestFatalReachesStderrBeforeExit(t *testing.T) {
	if os.Getenv(subprocessEnv) == "fatal" {
		runFatalChild()
		return // unreachable when logrus.Fatal works
	}
	stderr, exitCode := runChild(t, "fatal")
	require.NotEqual(t, 0, exitCode, "logrus.Fatal must produce a non-zero exit; child stderr was: %q", stderr)
	require.True(t, stderrContainsRecord(stderr, subprocessFatalMsg, "fatal"),
		"fatal record must reach stderr before os.Exit; child stderr was: %q", stderr)
}

// TestPanicReachesStderrBeforeExit mirrors the Fatal test for Panic. logrus
// fires hooks before issuing its panic; the bridge routes Panic-level records
// through SyncEmit so the leaf handler writes to stderr synchronously. The
// runtime's panic stack trace also goes to stderr; the assertion only cares
// that our JSON record arrived in the same stream.
func TestPanicReachesStderrBeforeExit(t *testing.T) {
	if os.Getenv(subprocessEnv) == "panic" {
		runPanicChild()
		return
	}
	stderr, exitCode := runChild(t, "panic")
	require.NotEqual(t, 0, exitCode, "logrus.Panic must produce a non-zero exit; child stderr was: %q", stderr)
	require.True(t, stderrContainsRecord(stderr, subprocessPanicMsg, "panic"),
		"panic record must reach stderr before the runtime panic; child stderr was: %q", stderr)
}

// TestFatalLabeledInPrettyOutput mirrors the Fatal durability test through
// the pretty handler chain: a post-Install logrus.Fatal must render the
// "FATAL" label in human-readable output, not a blank level column.
func TestFatalLabeledInPrettyOutput(t *testing.T) {
	if os.Getenv(subprocessEnv) == "fatal-pretty" {
		runPrettyChild(logrus.Fatal, subprocessFatalMsg)
		return
	}
	stderr, exitCode := runChild(t, "fatal-pretty")
	require.NotEqual(t, 0, exitCode, "logrus.Fatal must produce a non-zero exit; child stderr was: %q", stderr)
	require.True(t, prettyStderrContainsRecord(stderr, subprocessFatalMsg, "FATAL"),
		"pretty output must carry the FATAL label; child stderr was: %q", stderr)
}

// TestPanicLabeledInPrettyOutput mirrors TestFatalLabeledInPrettyOutput for
// Panic.
func TestPanicLabeledInPrettyOutput(t *testing.T) {
	if os.Getenv(subprocessEnv) == "panic-pretty" {
		runPrettyChild(logrus.Panic, subprocessPanicMsg)
		return
	}
	stderr, exitCode := runChild(t, "panic-pretty")
	require.NotEqual(t, 0, exitCode, "logrus.Panic must produce a non-zero exit; child stderr was: %q", stderr)
	require.True(t, prettyStderrContainsRecord(stderr, subprocessPanicMsg, "PANIC"),
		"pretty output must carry the PANIC label; child stderr was: %q", stderr)
}

// runChild forks the current test binary, asking it to run only the calling
// test with the subprocess env set to mode. It returns the child's stderr
// (captured) and exit code.
func runChild(t *testing.T, mode string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), subprocessEnv+"="+mode)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok {
		return stderr.String(), exitErr.ExitCode()
	}
	if err == nil {
		return stderr.String(), 0
	}
	t.Fatalf("subprocess failed to launch: %v", err)
	return stderr.String(), -1
}

func runFatalChild() {
	h, err := BuildHandler(os.Stderr, DefaultOptions())
	if err != nil {
		panic(err)
	}
	Install(h, slog.LevelInfo)
	logrus.Fatal(subprocessFatalMsg)
}

func runPanicChild() {
	h, err := BuildHandler(os.Stderr, DefaultOptions())
	if err != nil {
		panic(err)
	}
	Install(h, slog.LevelInfo)
	logrus.Panic(subprocessPanicMsg)
}

// runPrettyChild configures the default pretty handler chain (the controller,
// router, and tunnel operator default) over stderr and fires the given
// logrus call.
func runPrettyChild(logFn func(args ...any), msg string) {
	h, err := BuildHandlerForFormat(os.Stderr, DefaultOptions(), FormatPretty)
	if err != nil {
		panic(err)
	}
	Install(h, slog.LevelInfo)
	logFn(msg)
}

// prettyStderrContainsRecord scans the child's stderr for a pretty-format
// line that carries both the expected message and the expected level label.
func prettyStderrContainsRecord(stderr, msg, label string) bool {
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if strings.Contains(line, msg) && strings.Contains(line, label) {
			return true
		}
	}
	return false
}

// stderrContainsRecord scans the child's stderr for a JSON line that carries
// both the expected message and the expected level. The runtime panic trace
// and the testing harness's own output share the stream; non-JSON lines are
// ignored.
func stderrContainsRecord(stderr, msg, level string) bool {
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec["msg"] == msg && rec["level"] == level {
			return true
		}
	}
	return false
}
