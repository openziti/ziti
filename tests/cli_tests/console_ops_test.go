//go:build cli_tests

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
package cli_tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openziti/ziti/v2/ziti/cmd"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// runZiti drives the ziti command tree in-process with an explicit arg slice (so paths with
// spaces are not mangled) and returns command output plus the command error. Output is
// captured through a buffer wired into the command rather than by swapping os.Stdout, which
// avoids the os.Pipe deadlock that occurs when output exceeds the pipe buffer.
func runZiti(args ...string) (string, error) {
	var buf bytes.Buffer
	root := cmd.NewRootCommand(os.Stdin, &buf, &buf)
	root.SetArgs(args)
	root.SetOut(&buf)
	root.SetErr(&buf)

	err := root.Execute()
	return buf.String(), err
}

const sampleControllerConfig = `# top-level controller config
v: 3

# web listeners hosted by the controller
web:
  - name: client-management
    bindPoints:
      - interface: 0.0.0.0:1280
        address: localhost:1280
    # APIs bound to this listener
    apis:
      - binding: edge-management
        options: { }
      - binding: edge-client
        options: { }
  - name: dark-apis
    bindPoints:
      - interface: 0.0.0.0:1281
        address: localhost:1281
    apis:
      - binding: edge-client
        options: { }
`

// parsedConfig mirrors the slice of web listeners and their api bindings for assertions.
type parsedConfig struct {
	Web []struct {
		Name string `yaml:"name"`
		Apis []struct {
			Binding string `yaml:"binding"`
			Options struct {
				Path      string `yaml:"path"`
				Location  string `yaml:"location"`
				IndexFile string `yaml:"indexFile"`
			} `yaml:"options"`
		} `yaml:"apis"`
	} `yaml:"web"`
}

func parseConfig(t *testing.T, path string) parsedConfig {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg parsedConfig
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	return cfg
}

// spaCount returns how many spa bindings the named listener has, and the first spa options.
func spaForListener(t *testing.T, cfg parsedConfig, name string) (count int, path, location, indexFile string) {
	t.Helper()
	for _, l := range cfg.Web {
		if l.Name != name {
			continue
		}
		for _, a := range l.Apis {
			if a.Binding == "spa" {
				if count == 0 {
					path = a.Options.Path
					location = a.Options.Location
					indexFile = a.Options.IndexFile
				}
				count++
			}
		}
	}
	return count, path, location, indexFile
}

func writeSampleConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "controller.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(sampleControllerConfig), 0o644))
	return cfgPath
}

// fakeConsoleDir creates a directory that looks like an installed console (has index.html), so
// `ziti ops console configure` treats the assets as present and does not try to download.
func fakeConsoleDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "console")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html></html>"), 0o644))
	return dir
}

// Test_Console_Ops exercises `ziti ops console configure` and the no-network validation
// paths of `ziti ops console download`. It does not require a running controller, a built
// binary, or network access, so it stays a reliable guard that these commands keep working.
func Test_Console_Ops(t *testing.T) {
	t.Run("commands are registered", func(t *testing.T) {
		for _, args := range [][]string{
			{"ops", "console", "--help"},
			{"ops", "console", "download", "--help"},
			{"ops", "console", "configure", "--help"},
		} {
			_, err := runZiti(args...)
			require.NoError(t, err, "help for %v", args)
		}
	})

	t.Run("configure single listener by name", func(t *testing.T) {
		cfgPath := writeSampleConfig(t)
		location := fakeConsoleDir(t)

		_, err := runZiti("ops", "console", "configure", cfgPath, "--name", "client-management", "--location", location)
		require.NoError(t, err)

		cfg := parseConfig(t, cfgPath)

		count, path, loc, index := spaForListener(t, cfg, "client-management")
		require.Equal(t, 1, count, "client-management should have exactly one spa binding")
		require.Equal(t, "zac", path)
		require.Equal(t, location, loc)
		require.Equal(t, "index.html", index)

		darkCount, _, _, _ := spaForListener(t, cfg, "dark-apis")
		require.Equal(t, 0, darkCount, "dark-apis should be untouched")
	})

	t.Run("configure all is idempotent", func(t *testing.T) {
		cfgPath := writeSampleConfig(t)
		location := fakeConsoleDir(t)

		_, err := runZiti("ops", "console", "configure", cfgPath, "--all", "--location", location)
		require.NoError(t, err)
		// second run must update in place, not append a duplicate
		_, err = runZiti("ops", "console", "configure", cfgPath, "--all", "--location", location)
		require.NoError(t, err)

		cfg := parseConfig(t, cfgPath)
		for _, name := range []string{"client-management", "dark-apis"} {
			count, _, loc, _ := spaForListener(t, cfg, name)
			require.Equal(t, 1, count, "%s should have exactly one spa binding after two --all runs", name)
			require.Equal(t, location, loc)
		}
	})

	t.Run("configure multiple names", func(t *testing.T) {
		cfgPath := writeSampleConfig(t)
		location := fakeConsoleDir(t)

		_, err := runZiti("ops", "console", "configure", cfgPath,
			"--name", "client-management", "--name", "dark-apis", "--location", location)
		require.NoError(t, err)

		cfg := parseConfig(t, cfgPath)
		for _, name := range []string{"client-management", "dark-apis"} {
			count, _, _, _ := spaForListener(t, cfg, name)
			require.Equal(t, 1, count, "%s should have a spa binding", name)
		}
	})

	t.Run("configure custom path and index file", func(t *testing.T) {
		cfgPath := writeSampleConfig(t)
		location := fakeConsoleDir(t)

		_, err := runZiti("ops", "console", "configure", cfgPath, "--name", "client-management",
			"--location", location, "--path", "admin", "--index-file", "main.html")
		require.NoError(t, err)

		cfg := parseConfig(t, cfgPath)
		_, path, _, index := spaForListener(t, cfg, "client-management")
		require.Equal(t, "admin", path)
		require.Equal(t, "main.html", index)
	})

	t.Run("configure unknown name errors and leaves file unchanged", func(t *testing.T) {
		cfgPath := writeSampleConfig(t)
		before, err := os.ReadFile(cfgPath)
		require.NoError(t, err)

		_, err = runZiti("ops", "console", "configure", cfgPath, "--name", "nope", "--location", fakeConsoleDir(t))
		require.Error(t, err)

		after, readErr := os.ReadFile(cfgPath)
		require.NoError(t, readErr)
		require.Equal(t, string(before), string(after), "file must not change on error")
	})

	t.Run("configure rejects --all with --name", func(t *testing.T) {
		cfgPath := writeSampleConfig(t)
		_, err := runZiti("ops", "console", "configure", cfgPath, "--all", "--name", "client-management",
			"--location", fakeConsoleDir(t))
		require.Error(t, err)
	})

	t.Run("configure with no selector applies to all", func(t *testing.T) {
		cfgPath := writeSampleConfig(t)
		// -y assumes all when neither --all nor --name is given
		_, err := runZiti("ops", "console", "configure", cfgPath, "--location", fakeConsoleDir(t), "-y")
		require.NoError(t, err)

		cfg := parseConfig(t, cfgPath)
		for _, name := range []string{"client-management", "dark-apis"} {
			count, _, _, _ := spaForListener(t, cfg, name)
			require.Equal(t, 1, count, "%s should have a spa binding", name)
		}
	})

	t.Run("configure requires location with --yes", func(t *testing.T) {
		cfgPath := writeSampleConfig(t)
		// --yes cannot prompt for the location, so it must be supplied
		_, err := runZiti("ops", "console", "configure", cfgPath, "--all", "-y")
		require.Error(t, err)
	})

	t.Run("download requires location", func(t *testing.T) {
		_, err := runZiti("ops", "console", "download", "4.3.0")
		require.Error(t, err)
	})

	t.Run("download refuses a non-empty directory", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0o644))

		_, err := runZiti("ops", "console", "download", "4.3.0", "--location", dir)
		require.Error(t, err)

		// the pre-existing file must be untouched
		_, statErr := os.Stat(filepath.Join(dir, "keep.txt"))
		require.NoError(t, statErr)
	})

	// These hit the openziti/ziti-console GitHub releases over the network on purpose: they
	// are the guard that "latest" resolution and concrete-version downloads keep working.
	t.Run("download latest installs a usable console", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "console")

		_, err := runZiti("ops", "console", "download", "--location", dir)
		require.NoError(t, err)

		// a usable SPA must have an index.html
		_, statErr := os.Stat(filepath.Join(dir, "index.html"))
		require.NoError(t, statErr, "downloaded console must contain index.html")

		// the install records the resolved version
		version, vErr := os.ReadFile(filepath.Join(dir, ".version"))
		require.NoError(t, vErr)
		require.NotEmpty(t, string(version))
	})

	t.Run("download a specific version installs that version", func(t *testing.T) {
		const wantVersion = "4.3.0"
		dir := filepath.Join(t.TempDir(), "console")

		_, err := runZiti("ops", "console", "download", wantVersion, "--location", dir)
		require.NoError(t, err)

		_, statErr := os.Stat(filepath.Join(dir, "index.html"))
		require.NoError(t, statErr, "downloaded console must contain index.html")

		version, vErr := os.ReadFile(filepath.Join(dir, ".version"))
		require.NoError(t, vErr)
		require.Equal(t, wantVersion, strings.TrimSpace(string(version)))
	})
}
