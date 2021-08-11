/*
	Copyright NetFoundry, Inc.

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
	_ "embed"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"text/template"
)

const (
	optionCtrlListener  = "ctrlListener"
	defaultCtrlListener = "quic:0.0.0.0:6262"
)

var (
	createConfigControllerLong = templates.LongDesc(`
		Creates the controller config
`)

	createConfigControllerExample = templates.Examples(`
		# Create the controller config 
		ziti create config controller

		# Create the controller config with a particular ctrlListener
		ziti create config controller -ctrlListener quic:0.0.0.0:6262
	`)
)

//go:embed config_templates/controller.yml
var controllerConfigTemplate string

// CreateConfigControllerOptions the options for the create spring command
type CreateConfigControllerOptions struct {
	CreateConfigOptions

	OutputFile   string
	DatabaseFile string
	CtrlListener string
}

// NewCmdCreateConfigController creates a command object for the "create" command
func NewCmdCreateConfigController(p common.OptionsProvider) *cobra.Command {
	options := &CreateConfigControllerOptions{
		CreateConfigOptions: CreateConfigOptions{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:     "controller",
		Short:   "Create a controller config",
		Aliases: []string{"ctrl"},
		Long:    createConfigControllerLong,
		Example: createConfigControllerExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addFlags(cmd, "", defaultCtrlListener)

	return cmd
}

// Run implements the command
func (o *CreateConfigControllerOptions) Run() error {
	if o.CtrlListener == "" {
		return util.MissingOption(optionCtrlListener)
	}

	tmpl, err := template.New("controller-config").Parse(controllerConfigTemplate)
	if err != nil {
		return err
	}

	baseDir := filepath.Base(o.OutputFile)
	if baseDir != "." {
		if err := os.MkdirAll(baseDir, 0700); err != nil {
			return errors.Wrapf(err, "unable to create directory to house config file: %v", o.OutputFile)
		}
	}

	f, err := os.Create(o.OutputFile)
	if err != nil {
		return errors.Wrapf(err, "unable to create config file: %v", o.OutputFile)
	}
	defer func() { _ = f.Close() }()

	if err := tmpl.Execute(f, o); err != nil {
		return errors.Wrap(err, "unable to execute template")
	}

	return nil
}
