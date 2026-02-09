package cmd

import (
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestV1CommandsAvailableInV2 builds both the V1 and V2 command trees, walks
// every command in V1, and verifies that:
//  1. A command at the same path is reachable in V2 via Cobra's Find()
//     (the same resolution used at runtime).
//  2. The resolved command has the same Short description, Long description,
//     Example text, Aliases, and local flag names — i.e. it is functionally
//     the same command, just marked deprecated.
//
// Using Find() instead of a full-tree walk ensures we catch shadowed commands
// (e.g. two children with the same Use name under one parent, where only the
// first is reachable at runtime).
func TestV1CommandsAvailableInV2(t *testing.T) {
	v1Root := newRootCmd()
	NewV1CmdRoot(nil, io.Discard, io.Discard, v1Root)

	v2Root := newRootCmd()
	NewV2CmdRoot(nil, io.Discard, io.Discard, v2Root)

	// Collect every command path from V1.
	v1Commands := map[string]*cobra.Command{}
	collectCommands(v1Root, v1Commands)

	skipped := map[string]bool{
		"ziti create":        true, // V2 redesigned the create parent command
		"ziti create ca":     true, // conflicts with edge CA create in V2
		"ziti create config": true, // V2 uses this path for edge config entities; V1 children (router, controller, environment) are added as deprecated subcommands of the edge config command
	}

	for path, v1Cmd := range v1Commands {
		if skipped[path] {
			continue
		}

		t.Run(path, func(t *testing.T) {
			// Resolve V2 command the same way Cobra does at runtime.
			args := strings.Fields(path)[1:] // strip the "ziti" root prefix
			v2Cmd, remainder, err := v2Root.Find(args)
			require.NoError(t, err, "V1 command %q not found in V2 tree", path)
			require.Empty(t, remainder,
				"V1 command %q only partially resolved in V2 (resolved to %q with leftover args %v)",
				path, v2Cmd.CommandPath(), remainder)
			require.Equal(t, v1Cmd.Name(), v2Cmd.Name(),
				"V1 command %q resolved to wrong V2 command %q", path, v2Cmd.CommandPath())

			assert.Equal(t, v1Cmd.Short, v2Cmd.Short, "Short description mismatch for %q", path)
			assert.Equal(t, v1Cmd.Long, v2Cmd.Long, "Long description mismatch for %q", path)
			assert.Equal(t, v1Cmd.Example, v2Cmd.Example, "Example mismatch for %q", path)
			assert.Equal(t, v1Cmd.Aliases, v2Cmd.Aliases, "Aliases mismatch for %q", path)
			assert.Equal(t, localFlagNames(v1Cmd), localFlagNames(v2Cmd), "Local flags mismatch for %q", path)
		})
	}
}

// newRootCmd creates a bare root cobra.Command identical to the one used in
// production, suitable for passing to NewV1CmdRoot / NewV2CmdRoot.
func newRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ziti",
		Short: "ziti is a CLI for working with Ziti",
		Long: `
'ziti' is a CLI for working with a Ziti deployment.
`}
}

// collectCommands recursively walks the command tree rooted at cmd and
// populates the map keyed by CommandPath().
func collectCommands(cmd *cobra.Command, out map[string]*cobra.Command) {
	out[cmd.CommandPath()] = cmd
	for _, child := range cmd.Commands() {
		collectCommands(child, out)
	}
}

// localFlagNames returns a sorted list of local (non-inherited) flag names
// for the command, excluding the implicit --help flag.
func localFlagNames(cmd *cobra.Command) []string {
	var names []string
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "help" {
			names = append(names, f.Name)
		}
	})
	sort.Strings(names)
	return names
}
