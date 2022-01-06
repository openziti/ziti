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
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"strings"
	"text/template"
)

const (
	optionCtrlListener  = "ctrlListener"
	optionMgmtListener  = "mgmtListener"
	optionsDatabaseFile = "databaseFile"
)

var (
	createConfigControllerLong = templates.LongDesc(`
		Creates the controller config
`)

	createConfigControllerExample = templates.Examples(`
		# Create the controller config 
		ziti create config controller

		# Create the controller config with a particular ctrlListener
		ziti create config controller --ctrlListener tls:0.0.0.0:6262

		# Print the controller config to the console
		ziti create config controller --output stdout

		# Print the controller config to a file
		ziti create config controller --output <path to file>/<filename>.yaml
	`)
)

//go:embed config_templates/controller.yml
var controllerConfigTemplate string

// CreateConfigControllerOptions the options for the create spring command
type CreateConfigControllerOptions struct {
	CreateConfigOptions

	CtrlListener string
	MgmtListener string
}

//type ControllerConfigValues struct {
//	*ConfigValues
//
//	CtrlListener                 string
//	MgmtListener                 string
//	ZitiHome                     string
//	Hostname                     string
//	ZitiFabMgmtPort              string
//	ZitiEdgeCtrlAPI              string
//	ZitiSigningIntermediateName  string
//	ZitiEdgeCtrlPort             string
//	ZitiCtrlRawname              string
//	ZitiEdgeCtrlIntermediateNameVarName string
//	ZitiEdgeCtrlHostname         string
//}

// NewCmdCreateConfigController creates a command object for the "create" command
func NewCmdCreateConfigController(data *ConfigTemplateValues) *cobra.Command {
	controllerOptions := &CreateConfigControllerOptions{}

	// controllerOptions := &CreateConfigControllerOptions{
	// 	CreateConfigOptions: configOptions,
	// }

	cmd := &cobra.Command{
		Use:     "controller",
		Short:   "Create a controller config",
		Aliases: []string{"ctrl"},
		Long:    createConfigControllerLong,
		Example: createConfigControllerExample,
		PreRun: func(cmd *cobra.Command, args []string) {
			// Setup logging
			var logOut *os.File
			if controllerOptions.Verbose {
				logrus.SetLevel(logrus.DebugLevel)
				// Only print log to stdout if not printing config to stdout
				if strings.ToLower(controllerOptions.Output) != "stdout" {
					logOut = os.Stdout
				} else {
					logOut = os.Stderr
				}
				logrus.SetOutput(logOut)
			}

			// Update controller specific values with configOptions passed in
			// data.CtrlListener = controllerOptions.CtrlListener
			// data.MgmtListener = controllerOptions.MgmtListener
		},
		Run: func(cmd *cobra.Command, args []string) {
			controllerOptions.Cmd = cmd
			controllerOptions.Args = args
			err := controllerOptions.run(data)
			cmdhelper.CheckErr(err)
		},
		PostRun: func(cmd *cobra.Command, args []string) {
			// Reset log output after run completes
			logrus.SetOutput(os.Stdout)
		},
	}
	controllerOptions.addCreateFlags(cmd)
	controllerOptions.addFlags(cmd)

	return cmd
}

func (options *CreateConfigControllerOptions) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&options.CtrlListener, optionCtrlListener, "tls:0.0.0.0:6262", "address of the config controller listener")
	cmd.Flags().StringVar(&options.DatabaseFile, optionsDatabaseFile, "ctrl.db", "location of the database file")
	cmd.Flags().StringVar(&options.MgmtListener, optionMgmtListener, "tls:127.0.0.1:10000", "address of the config management listener")
}

// run implements the command
func (options *CreateConfigControllerOptions) run(data *ConfigTemplateValues) error {

	tmpl, err := template.New("controller-config").Parse(controllerConfigTemplate)
	if err != nil {
		return err
	}

	// TODO: Do we want to create the path if it doesn't exist?
	//baseDir := filepath.Dir(options.Output)
	//if baseDir != "." {
	//	if err := os.MkdirAll(baseDir, 0700); err != nil {
	//		return errors.Wrapf(err, "unable to create directory to house config file: %v", options.Output)
	//	}
	//}

	var f *os.File
	if strings.ToLower(options.Output) != "stdout" {
		f, err = os.Create(options.Output)
		logrus.Debugf("Created output file: %s", options.Output)
		if err != nil {
			return errors.Wrapf(err, "unable to create config file: %s", options.Output)
		}
	} else {
		f = os.Stdout
	}
	defer func() { _ = f.Close() }()

	if err := tmpl.Execute(f, data); err != nil {
		return errors.Wrap(err, "unable to execute template")
	}

	logrus.Debugf("Controller configuration generated successfully and written to: %s", options.Output)

	return nil
}
