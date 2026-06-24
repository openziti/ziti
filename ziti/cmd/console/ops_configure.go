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

package console

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type ConfigureOptions struct {
	Out        io.Writer
	Err        io.Writer
	In         io.Reader
	ConfigFile string
	All        bool
	Names      []string
	Location   string
	Path       string
	IndexFile  string
	Verbose    bool
	Yes        bool
}

func newConfigureCmd(out, errOut io.Writer) *cobra.Command {
	options := &ConfigureOptions{
		Out:       out,
		Err:       errOut,
		In:        os.Stdin,
		Path:      "zac",
		IndexFile: "index.html",
	}

	cmd := &cobra.Command{
		Use:   "configure <controller-config-file>",
		Short: "Add or update the console (spa) binding in a controller config file",
		Long: `Edits a controller config (YAML) file in place, adding or updating the console "spa"
web-listener binding so the controller serves the console from a directory on disk.

Select which web listeners to update with --all, or with one or more --name flags. The file's
comments and structure are preserved.

Examples:
  # update every web listener
  ziti ops console configure ./controller.yaml --all --location /opt/openziti/share/console

  # update a single listener (quickstart default)
  ziti ops console configure ./controller.yaml --name client-management \
      --location /opt/openziti/share/console

  # update several named listeners
  ziti ops console configure ./controller.yaml --name client-management --name dark-apis \
      --location /opt/openziti/share/console`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.ConfigFile = args[0]
			return options.Run()
		},
	}

	cmd.Flags().BoolVar(&options.All, "all", false, "Apply to every web listener in the config")
	cmd.Flags().StringArrayVar(&options.Names, "name", nil, "Name of a web listener to apply to; repeat for multiple")
	cmd.Flags().StringVarP(&options.Location, "location", "l", "", "Console assets directory the controller should serve; prompted for if omitted")
	cmd.Flags().StringVar(&options.Path, "path", options.Path, "URL path the console is served under")
	cmd.Flags().StringVar(&options.IndexFile, "index-file", options.IndexFile, "SPA index file served as the fallback")
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "v", false, "Print the resulting binding YAML section")
	cmd.Flags().BoolVarP(&options.Yes, "yes", "y", false, "Answer yes to prompts (use the location given and download latest ZAC if absent)")

	return cmd
}

func (o *ConfigureOptions) Run() error {
	o.Path = normalizeConsolePath(o.Path)
	if o.Path == "" {
		return fmt.Errorf("--path must not be empty")
	}

	reader := bufio.NewReader(o.In)
	if err := o.ensureLocationAndAssets(reader); err != nil {
		return err
	}
	c := &bindingConfigurator{
		out:        o.Out,
		in:         o.In,
		reader:     reader,
		configFile: o.ConfigFile,
		all:        o.All,
		names:      o.Names,
		binding:    "spa",
		matchKey:   "path",
		matchValue: o.Path,
		verbose:    o.Verbose,
		yes:        o.Yes,
		applyOptions: func(options *yaml.Node) {
			setMapScalar(options, "path", o.Path)
			setMapScalar(options, "location", o.Location)
			setMapScalar(options, "indexFile", o.IndexFile)
		},
	}
	return c.run()
}

// ensureLocationAndAssets resolves the console assets directory, prompting for it when not
// supplied, and offers to download ZAC into it when none is present. With --yes it never
// prompts: it requires --location and downloads the latest if assets are missing.
func (o *ConfigureOptions) ensureLocationAndAssets(reader *bufio.Reader) error {
	if o.Location == "" {
		if o.Yes {
			return fmt.Errorf("--location is required (cannot prompt for it with --yes)")
		}
		loc, err := prompt(o.Out, reader, "--location flag not supplied. Where are the console assets located? ")
		if err != nil {
			return err
		}
		o.Location = loc
		if o.Location == "" {
			return fmt.Errorf("a console assets location is required")
		}
	}

	abs, err := filepath.Abs(o.Location)
	if err != nil {
		return fmt.Errorf("cannot resolve location '%s' to an absolute path: %w", o.Location, err)
	}
	o.Location = abs

	// Assets already present, nothing to download.
	if validateAssetsDir(o.Location) == nil {
		return nil
	}

	if !o.Yes {
		ok, err := promptYesNo(o.Out, reader, fmt.Sprintf("No console found at '%s'. Download it? [Y/n]: ", o.Location), true)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("no console assets at '%s'; supply a populated --location or allow the download", o.Location)
		}
	}

	// Never wipe a non-empty directory that is not a prior console install.
	if entries, err := os.ReadDir(o.Location); err == nil && len(entries) > 0 && installedVersion(o.Location) == "" {
		return fmt.Errorf("location '%s' is not empty and is not a console install; choose an empty directory", o.Location)
	}

	version := "latest"
	if !o.Yes {
		v, err := prompt(o.Out, reader, "Version to download [latest]: ")
		if err != nil {
			return err
		}
		if v != "" {
			version = v
		}
	}

	// Be lenient about a leading "v": the release tag prefix already supplies it, so strip it.
	version = normalizeVersion(version)
	if version == "" || strings.EqualFold(version, "latest") {
		resolved, err := resolveLatestVersion()
		if err != nil {
			return fmt.Errorf("failed to resolve latest ZAC version: %w", err)
		}
		version = resolved
	}

	_, _ = fmt.Fprintf(o.Out, "\nDownloading ZAC %s from %s\n  to %s ...\n", version, downloadURL(version), o.Location)
	sum, err := downloadRelease(version, o.Location)
	if err != nil {
		return err
	}
	if err = validateAssetsDir(o.Location); err != nil {
		return fmt.Errorf("downloaded archive did not contain a usable console: %w", err)
	}
	_, _ = fmt.Fprintf(o.Out, "Downloaded ZAC %s (sha256 %s)\n", version, sum)
	return nil
}

// normalizeConsolePath strips surrounding whitespace and any leading or trailing slashes so
// "/local", "local/", "//local//", and "local" all resolve to the same path segment.
func normalizeConsolePath(p string) string {
	return strings.Trim(strings.TrimSpace(p), "/\\")
}

func prompt(out io.Writer, reader *bufio.Reader, label string) (string, error) {
	_, _ = fmt.Fprint(out, label)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptYesNo(out io.Writer, reader *bufio.Reader, label string, defaultYes bool) (bool, error) {
	ans, err := prompt(out, reader, label)
	if err != nil {
		return false, err
	}
	ans = strings.ToLower(ans)
	if ans == "" {
		return defaultYes, nil
	}
	return ans == "y" || ans == "yes", nil
}

// promptForListeners asks, per named web listener, whether to apply the binding, recording the
// confirmed names. It is used when neither --all nor --name was given (and not --yes).
func (c *bindingConfigurator) promptForListeners(web *yaml.Node) error {
	_, _ = fmt.Fprintf(c.out, "\nNeither --all nor --name was given. Choose listeners for the %s binding:\n\n", c.binding)
	for _, listener := range web.Content {
		if listener.Kind != yaml.MappingNode {
			continue
		}
		name := scalarValue(mapValue(listener, "name"))
		if name == "" {
			continue
		}
		ok, err := promptYesNo(c.out, c.reader,
			fmt.Sprintf("  Apply the %s binding to the web listener named '%s'? [Y/n]: ", c.binding, name), true)
		if err != nil {
			return err
		}
		if ok {
			c.names = append(c.names, name)
		}
	}
	_, _ = fmt.Fprintln(c.out)
	if len(c.names) == 0 {
		return fmt.Errorf("no web listeners selected")
	}
	return nil
}

// bindingConfigurator adds or updates a single web-listener api binding across the selected
// listeners of a controller config, preserving comments and structure.
type bindingConfigurator struct {
	out        io.Writer
	in         io.Reader
	reader     *bufio.Reader
	configFile string
	all        bool
	names      []string
	binding    string
	// matchKey/matchValue disambiguate multiple bindings of the same name by an options field
	// (e.g. spa bindings keyed by "path"), so distinct paths append and a repeated path updates.
	// matchKey "" matches on binding name alone.
	matchKey   string
	matchValue string
	verbose    bool
	yes        bool
	// applyOptions mutates the binding's options mapping node. nil leaves an empty `options: {}`.
	applyOptions func(options *yaml.Node)
}

func (c *bindingConfigurator) run() error {
	if c.reader == nil {
		c.reader = bufio.NewReader(c.in)
	}
	if c.all && len(c.names) > 0 {
		return fmt.Errorf("specify either --all or --name, not both")
	}

	data, err := os.ReadFile(c.configFile)
	if err != nil {
		return fmt.Errorf("failed to read config '%s': %w", c.configFile, err)
	}

	var doc yaml.Node
	if err = yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse config '%s': %w", c.configFile, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("config '%s' is not a YAML mapping", c.configFile)
	}
	root := doc.Content[0]

	web := mapValue(root, "web")
	if web == nil || web.Kind != yaml.SequenceNode {
		return fmt.Errorf("config '%s' has no web listeners (no `web:` sequence)", c.configFile)
	}

	// With no selector, default to all. When interactive, confirm per listener.
	if !c.all && len(c.names) == 0 {
		if c.yes {
			c.all = true
		} else if err = c.promptForListeners(web); err != nil {
			return err
		}
	}

	// Track requested names so we can report any that were not found.
	pending := map[string]bool{}
	for _, n := range c.names {
		pending[n] = true
	}

	type touched struct {
		label  string
		action string
		node   *yaml.Node
	}
	var changes []touched
	changedAny := false

	matched := 0
	for _, listener := range web.Content {
		if listener.Kind != yaml.MappingNode {
			continue
		}
		name := scalarValue(mapValue(listener, "name"))
		if !c.selected(name) {
			continue
		}
		delete(pending, name)
		matched++

		action, node, changed := ensureBinding(listener, c.binding, c.matchKey, c.matchValue, c.applyOptions)
		if changed {
			changedAny = true
		}
		label := name
		if label == "" {
			label = "(unnamed)"
		}
		desc := c.binding
		if c.matchKey != "" {
			desc = fmt.Sprintf("%s (%s %q)", c.binding, c.matchKey, c.matchValue)
		}
		_, _ = fmt.Fprintf(c.out, "%s %s binding on web listener '%s'\n", action, desc, label)
		changes = append(changes, touched{label: label, action: action, node: node})
	}

	if len(pending) > 0 {
		missing := make([]string, 0, len(pending))
		for _, n := range c.names {
			if pending[n] {
				missing = append(missing, fmt.Sprintf("%q", n))
			}
		}
		return fmt.Errorf("no web listener named %s found in %s", strings.Join(missing, ", "), c.configFile)
	}
	if matched == 0 {
		return fmt.Errorf("no matching web listeners found in %s", c.configFile)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err = enc.Encode(&doc); err != nil {
		return fmt.Errorf("failed to render updated config: %w", err)
	}
	_ = enc.Close()

	// Reuse the original mode so a config holding secrets isn't widened to the default 0644.
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(c.configFile); statErr == nil {
		mode = info.Mode().Perm()
	}

	// CreateTemp in the same dir gives an O_EXCL, non-predictable path (a symlink can't redirect the
	// write) and keeps the rename on one filesystem.
	tmpFile, err := os.CreateTemp(filepath.Dir(c.configFile), ".console-config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err = tmpFile.Write(buf.Bytes()); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write updated config: %w", err)
	}
	if err = tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to flush updated config: %w", err)
	}
	if err = os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("failed to set permissions on updated config: %w", err)
	}
	if err = os.Rename(tmpName, c.configFile); err != nil {
		return fmt.Errorf("failed to replace '%s': %w", c.configFile, err)
	}

	added, updated := 0, 0
	for _, ch := range changes {
		if ch.action == "added" {
			added++
		} else {
			updated++
		}
	}
	_, _ = fmt.Fprintf(c.out, "\nsaved %s (%d added, %d updated across %d web listener(s))\n",
		c.configFile, added, updated, matched)

	if c.verbose {
		for _, ch := range changes {
			section, rErr := renderSection(ch.node)
			if rErr != nil {
				continue
			}
			_, _ = fmt.Fprintf(c.out, "\n%s:\n%s", ch.label, section)
		}
	}

	if changedAny {
		_, _ = fmt.Fprintln(c.out, "restart your controller to pick up changes")
	}
	return nil
}

func (c *bindingConfigurator) selected(name string) bool {
	if c.all {
		return true
	}
	for _, n := range c.names {
		if n == name {
			return true
		}
	}
	return false
}

// ensureBinding makes sure the listener's apis list holds a binding of the given name with the
// supplied options, updating an existing one or appending a new one. When matchKey is set, a
// binding matches only if its options[matchKey] equals matchValue, so several bindings of the
// same name (e.g. spa bindings on different paths) coexist and only the matching one is updated.
// It returns "added" or "updated", the binding node that was written, and whether the file
// content actually changed (an add, or an update that altered the options).
func ensureBinding(listener *yaml.Node, binding, matchKey, matchValue string, applyOptions func(*yaml.Node)) (string, *yaml.Node, bool) {
	apis := mapValue(listener, "apis")
	if apis == nil {
		apis = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		setMapNode(listener, "apis", apis)
	} else if apis.Kind != yaml.SequenceNode {
		// apis present but null or empty, so coerce it into a sequence.
		apis.Kind = yaml.SequenceNode
		apis.Tag = "!!seq"
		apis.Value = ""
		apis.Content = nil
	}

	for _, api := range apis.Content {
		if api.Kind != yaml.MappingNode {
			continue
		}
		if scalarValue(mapValue(api, "binding")) != binding {
			continue
		}
		if matchKey != "" && scalarValue(mapValue(mapValue(api, "options"), matchKey)) != matchValue {
			continue
		}
		before, _ := renderSection(api)
		applyBindingOptions(api, applyOptions)
		after, _ := renderSection(api)
		return "updated", api, before != after
	}

	node := newBindingNode(binding, applyOptions)
	apis.Content = append(apis.Content, node)
	return "added", node, true
}

func newBindingNode(binding string, applyOptions func(*yaml.Node)) *yaml.Node {
	api := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setMapScalar(api, "binding", binding)
	options := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	setMapNode(api, "options", options)
	if applyOptions != nil {
		applyOptions(options)
	}
	return api
}

func applyBindingOptions(api *yaml.Node, applyOptions func(*yaml.Node)) {
	if applyOptions == nil {
		return
	}
	options := mapValue(api, "options")
	if options == nil || options.Kind != yaml.MappingNode {
		options = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		setMapNode(api, "options", options)
	}
	applyOptions(options)
}

// renderSection encodes a single binding node as a YAML list item indented two spaces, for
// display under its web listener name.
func renderSection(node *yaml.Node) (string, error) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{node}}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(seq); err != nil {
		return "", err
	}
	_ = enc.Close()

	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// mapValue returns the value node for key in a mapping node, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalarValue(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}

// setMapNode sets key to val in a mapping, replacing an existing value or appending.
func setMapNode(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, scalarNode(key), val)
}

// setMapScalar sets key to a string scalar, updating in place to preserve any comments.
func setMapScalar(m *yaml.Node, key, value string) {
	if v := mapValue(m, key); v != nil {
		v.Kind = yaml.ScalarNode
		v.Tag = "!!str"
		v.Value = value
		v.Content = nil
		return
	}
	m.Content = append(m.Content, scalarNode(key), scalarNode(value))
}

func scalarNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}
