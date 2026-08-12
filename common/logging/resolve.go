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
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Environment variables that force non-JSON (pretty) output when the
// --log-formatter flag is unset. EnvNoJson is the current name; EnvNoJsonLegacy
// is the pfxlog-era name, still honored so existing deployments and test
// harnesses keep working.
const (
	EnvNoJson       = "ZITI_LOG_NO_JSON"
	EnvNoJsonLegacy = "PFXLOG_NO_JSON"
)

// ResolveFormat returns the effective --log-formatter value for the given flag
// value and output destination.
//
// An explicit, non-empty flagValue always wins and is returned unchanged. When
// the flag is unset, the format is chosen by terminal detection, the way ziti
// has historically chosen it: ZITI_LOG_NO_JSON (or the deprecated
// PFXLOG_NO_JSON) forces pretty output; otherwise a terminal out gets pretty
// output and a non-terminal (piped or redirected) out gets JSON, so production
// processes default to the machine-parseable shape.
//
// out is the writer the handler will target (os.Stderr for ziti binaries). A
// nil out, or one whose fd is not a terminal, counts as non-terminal.
func ResolveFormat(flagValue string, out *os.File) string {
	return resolveFormat(flagValue, noJsonEnv(), isTerminal(out))
}

// resolveFormat is the pure decision behind ResolveFormat, split out so the
// precedence rules can be tested without touching the environment or a tty.
func resolveFormat(flagValue string, noJson, isTerminal bool) string {
	if flagValue != "" {
		return flagValue
	}
	if noJson {
		return FormatPretty
	}
	if isTerminal {
		return FormatPretty
	}
	return FormatJSON
}

// noJsonEnv reports whether either NO_JSON environment variable is set to a
// truthy value, preferring the current name over the deprecated one.
func noJsonEnv() bool {
	return envBool(EnvNoJson) || envBool(EnvNoJsonLegacy)
}

// envBool parses a boolean environment variable with strconv.ParseBool on its
// lowercased value, matching pfxlog's semantics. An unset variable is false; an
// unparsable value warns to stderr and is treated as false.
func envBool(name string) bool {
	v := strings.ToLower(os.Getenv(name))
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error parsing environment variable '%s' (%v)\n", name, err)
		return false
	}
	return b
}

// isTerminal reports whether out is a terminal. A nil out is not a terminal.
func isTerminal(out *os.File) bool {
	return out != nil && term.IsTerminal(int(out.Fd()))
}
