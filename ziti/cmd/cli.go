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

package cmd

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/spf13/cobra"
)

// NewCliCmd creates the cli command
func NewCliCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "cli",
		Short:  "CLI configuration commands",
		Hidden: true,
	}

	cmd.AddCommand(newCliSetCmd(out, errOut))
	cmd.AddCommand(newCliGetCmd(out, errOut))
	cmd.AddCommand(newCliRemoveCmd(out, errOut))

	return cmd
}

// newCliSetCmd creates the 'set' subcommand
func newCliSetCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set CLI configuration values",
	}

	cmd.AddCommand(newSetLayoutCmd(out, errOut))
	cmd.AddCommand(newSetAliasCmd(out, errOut))

	return cmd
}

// newCliGetCmd creates the 'get' subcommand
func newCliGetCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get CLI configuration values",
	}

	cmd.AddCommand(newGetLayoutCmd(out, errOut))
	cmd.AddCommand(newGetAliasCmd(out, errOut))

	return cmd
}

// newCliRemoveCmd creates the 'remove' subcommand
func newCliRemoveCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove CLI configuration values",
	}

	cmd.AddCommand(newRemoveAliasCmd(out, errOut))

	return cmd
}

// setLayoutOptions are the flags for set layout command
type setLayoutOptions struct {
	api.Options
}

// newSetLayoutCmd creates the 'set layout' command
func newSetLayoutCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &setLayoutOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "layout <1|2>",
		Short: "Set the CLI layout version",
		Long:  "Set the CLI layout version. Version 1 is the legacy layout, version 2 is the new layout.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return options.Run()
		},
	}

	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements the set layout command
func (o *setLayoutOptions) Run() error {
	layoutStr := o.Args[0]
	layout, err := strconv.Atoi(layoutStr)
	if err != nil {
		return fmt.Errorf("invalid layout version: %s (must be 1 or 2)", layoutStr)
	}

	if layout < 1 || layout > 2 {
		return fmt.Errorf("invalid layout version: %d (must be 1 or 2)", layout)
	}

	config, configFile, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	config.Layout = layout
	o.Printf("Setting CLI layout to %d in %s\n", layout, configFile)
	return util.PersistRestClientConfig(config)
}

// getLayoutOptions are the flags for get layout command
type getLayoutOptions struct {
	api.Options
}

// newGetLayoutCmd creates the 'get layout' command
func newGetLayoutCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &getLayoutOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "layout",
		Short: "Get the current CLI layout version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return options.Run()
		},
	}

	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements the get layout command
func (o *getLayoutOptions) Run() error {
	config, _, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	layout := config.Layout
	if layout == 0 {
		layout = 1 // default to 1 if not set
	}

	o.Printf("%d\n", layout)
	return nil
}

// setAliasOptions are the flags for set alias command
type setAliasOptions struct {
	api.Options
}

// newSetAliasCmd creates the 'set alias' command
func newSetAliasCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &setAliasOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "alias <name> <target>",
		Short: "Set a command alias",
		Long: `Set a command alias. The alias will be available as 'ziti <name>' and will
delegate to 'ziti <target>', passing along all subsequent arguments.

Example:
  ziti cli set alias agent "ops agent"

This creates an alias so that 'ziti agent status' becomes 'ziti ops agent status'.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return options.Run()
		},
	}

	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements the set alias command
func (o *setAliasOptions) Run() error {
	aliasName := o.Args[0]
	target := o.Args[1]

	// Validate alias name doesn't contain spaces
	if strings.ContainsAny(aliasName, " \t") {
		return fmt.Errorf("alias name cannot contain spaces: %s", aliasName)
	}

	config, configFile, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	if config.Aliases == nil {
		config.Aliases = make(map[string]string)
	}

	config.Aliases[aliasName] = target
	o.Printf("Setting alias '%s' -> 'ziti %s' in %s\n", aliasName, target, configFile)
	return util.PersistRestClientConfig(config)
}

// getAliasOptions are the flags for get alias command
type getAliasOptions struct {
	api.Options
}

// newGetAliasCmd creates the 'get alias' command
func newGetAliasCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &getAliasOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "alias [name]",
		Short: "Get command aliases",
		Long:  "Get all command aliases, or a specific alias if a name is provided.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return options.Run()
		},
	}

	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements the get alias command
func (o *getAliasOptions) Run() error {
	config, _, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	if config.Aliases == nil || len(config.Aliases) == 0 {
		if len(o.Args) > 0 {
			return fmt.Errorf("alias '%s' not found", o.Args[0])
		}
		o.Println("No aliases defined")
		return nil
	}

	// If a specific alias is requested
	if len(o.Args) > 0 {
		aliasName := o.Args[0]
		if target, ok := config.Aliases[aliasName]; ok {
			o.Printf("%s -> ziti %s\n", aliasName, target)
			return nil
		}
		return fmt.Errorf("alias '%s' not found", aliasName)
	}

	// List all aliases
	var names []string
	for name := range config.Aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		o.Printf("%s -> ziti %s\n", name, config.Aliases[name])
	}
	return nil
}

// removeAliasOptions are the flags for remove alias command
type removeAliasOptions struct {
	api.Options
}

// newRemoveAliasCmd creates the 'remove alias' command
func newRemoveAliasCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &removeAliasOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "alias <name>",
		Short: "Remove a command alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return options.Run()
		},
	}

	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements the remove alias command
func (o *removeAliasOptions) Run() error {
	aliasName := o.Args[0]

	config, configFile, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	if config.Aliases == nil {
		return fmt.Errorf("alias '%s' not found", aliasName)
	}

	if _, ok := config.Aliases[aliasName]; !ok {
		return fmt.Errorf("alias '%s' not found", aliasName)
	}

	delete(config.Aliases, aliasName)
	o.Printf("Removed alias '%s' from %s\n", aliasName, configFile)
	return util.PersistRestClientConfig(config)
}
