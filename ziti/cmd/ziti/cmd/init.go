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
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/spf13/cobra"
	"io"
)

// InitOptions the flags for running init
type InitOptions struct {
	CommonOptions
	Flags InitFlags
}

type InitFlags struct {
	Component string
}

var (
	initLong = templates.LongDesc(`
		This command installs a Ziti configuration template for a selected Ziti component
	`)

	initExample = templates.Examples(`
		ziti init
	`)
)

// NewCmdInit creates a command object for the generic "init" action
func NewCmdInit(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InitOptions{
		CommonOptions: CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Initialize a Ziti Configuration",
		Long:    initLong,
		Example: initExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.AddCommonFlags(cmd)

	options.addInitFlags(cmd)
	return cmd
}

func (options *InitOptions) addInitFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&options.Flags.Component, "component", "", "", "Ziti component being initialized.")
}

func (o *InitOptions) Run() error {

	var err error
	if o.Flags.Component == "" {
		o.Flags.Component, err = o.GetZitiComponent(o.Flags.Component)
		if err != nil {
			return err
		}
	}

	err = o.installRequirements("")
	if err != nil {
		return err
	}

	switch o.Flags.Component {
	case c.ZITI_CONTROLLER:
		err = o.createInitialControllerConfig()
	case c.ZITI_ROUTER:
		return nil
	case c.ZITI:
		err = o.createInitialZitiConfig()
	default:
	}

	if err != nil {
		return err
	}

	return nil
}
