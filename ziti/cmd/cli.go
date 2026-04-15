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
	"strconv"

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

	return cmd
}

// newCliSetCmd creates the 'set' subcommand
func newCliSetCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set CLI configuration values",
	}

	cmd.AddCommand(newSetLayoutCmd(out, errOut))

	return cmd
}

// newCliGetCmd creates the 'get' subcommand
func newCliGetCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get CLI configuration values",
	}

	cmd.AddCommand(newGetLayoutCmd(out, errOut))

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

	if layout == 2 {
		config.LayoutNoticeShown = false
		o.Printf("NOTE: CLI layout 2 is experimental and will likely change before being finalized.\n")
	}

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

