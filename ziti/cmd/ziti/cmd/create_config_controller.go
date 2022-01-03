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
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
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

	ZitiHome                     string
	Hostname                     string
	CtrlListener                 string
	MgmtListener                 string
	ZitiFabMgmtPort              string
	ZitiEdgeCtrlAPI              string
	ZitiSigningIntermediateName  string
	ZitiEdgeCtrlPort             string
	ZitiCtrlRawname              string
	ZitiEdgeCtrlIntermediateName string
	ZitiEdgeCtrlHostname         string
}

// NewCmdCreateConfigController creates a command object for the "create" command
func NewCmdCreateConfigController(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	//options := &CreateConfigControllerOptions{
	//	CreateConfigOptions: p,
	//}

	options := &CreateConfigControllerOptions{
		CreateConfigOptions: CreateConfigOptions{
			CommonOptions: common.CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "controller",
		Short:   "Create a controller config",
		Aliases: []string{"ctrl"},
		Long:    createConfigControllerLong,
		Example: createConfigControllerExample,
		PreRun: func(cmd *cobra.Command, args []string) {
			// TODO: Might not use this
		},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := run(options)
			cmdhelper.CheckErr(err)
		},
		PostRun: func(cmd *cobra.Command, args []string) {
			// Reset log output after run completes
			logrus.SetOutput(os.Stdout)
		},
	}

	options.addFlags(cmd)

	return cmd
}

func (options *CreateConfigControllerOptions) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&options.CtrlListener, optionCtrlListener, "tls:0.0.0.0:6262", "address of the config controller listener")
	cmd.Flags().StringVar(&options.DatabaseFile, optionsDatabaseFile, "ctrl.db", "location of the database file")
	cmd.Flags().StringVar(&options.MgmtListener, optionMgmtListener, "tls:127.0.0.1:10000", "address of the config management listener")
}

// run implements the command
func run(options *CreateConfigControllerOptions) error {
	outputFile := options.Output
	//outputFile := viper.GetString(optionOutput)
	isVerbose := viper.GetBool(optionVerbose)
	fmt.Printf("###run() output is: `%s`\n", outputFile)

	// Setup logging
	var logOut *os.File
	if isVerbose {
		logrus.SetLevel(logrus.DebugLevel)
		// Only print log to stdout if not printing config to stdout
		if strings.ToLower(options.Output) != "stdout" {
			logOut = os.Stdout
		} else {
			logOut = os.Stderr
		}
		logrus.SetOutput(logOut)
	}

	tmpl, err := template.New("controller-config").Parse(controllerConfigTemplate)
	if err != nil {
		return err
	}

	//baseDir := filepath.Dir(options.Output)
	//if baseDir != "." {
	//	if err := os.MkdirAll(baseDir, 0700); err != nil {
	//		return errors.Wrapf(err, "unable to create directory to house config file: %v", options.Output)
	//	}
	//}

	populateEnvVars(options)

	var f *os.File
	if strings.ToLower(options.Output) != "stdout" {
		f, err = os.Create(outputFile)
		logrus.Debugf("Created output file: %s", options.Output)
		if err != nil {
			return errors.Wrapf(err, "unable to create config file: %s", options.Output)
		}
	} else {
		f = os.Stdout
	}
	defer func() { _ = f.Close() }()

	//// If using stdout for config, send logs to stderr
	//if strings.ToLower(options.Output) == "stdout" {
	//	logrus.SetOutput(os.Stderr)
	//}

	if err := tmpl.Execute(f, options); err != nil {
		return errors.Wrap(err, "unable to execute template")
	}

	logrus.Debugf("Controller configuration generated successfully and written to: %s", options.Output)

	return nil
}

func populateEnvVars(options *CreateConfigControllerOptions) {

	// Get and add hostname to the params
	hostname, err := os.Hostname()
	handleVariableError(err, "hostname")

	// Get and add ziti home to the params
	zitiHome, err := cmdhelper.GetZitiHome()
	handleVariableError(err, cmdhelper.ZitiHomeVarName)

	// Get Ziti Controller Rawname
	zitiCtrlRawname, err := cmdhelper.GetZitiCtrlRawname()
	handleVariableError(err, cmdhelper.ZitiCtrlRawnameVarName)

	// Get Ziti fabric ctrl port
	zitiFabCtrlPort, err := cmdhelper.GetZitiFabCtrlPort()
	handleVariableError(err, cmdhelper.ZitiFabCtrlPortVarName)

	// Get Ziti PKI path
	zitiPKI, err := cmdhelper.GetZitiPKI()
	handleVariableError(err, cmdhelper.ZitiPKIVarName)

	// Get Ziti Controller Intermediate Name
	zitiCtrlIntName, err := cmdhelper.GetZitiCtrlIntermediateName()
	handleVariableError(err, cmdhelper.ZitiCtrlIntermediateNameVarName)

	// Get Ziti Controller Hostname
	zitiCtrlHostname, err := cmdhelper.GetZitiCtrlHostname()
	handleVariableError(err, cmdhelper.ZitiCtrlHostnameVarName)

	// Get Ziti Fabric Management Port
	zitiFabMgmtPort, err := cmdhelper.GetZitiFabMgmtPort()
	handleVariableError(err, cmdhelper.ZitiFabMgmtPortVarName)

	// Get Ziti Edge Controller API
	zitiEdgeCtrlAPI, err := cmdhelper.GetZitiEdgeControllerAPI()
	handleVariableError(err, cmdhelper.ZitiEdgeCtrlAPIVarName)

	// Get Ziti Signing Intermediate Name
	zitiSigningIntermediateName, err := cmdhelper.GetZitiSigningIntermediateName()
	handleVariableError(err, cmdhelper.ZitiSigningIntermediateNameVarName)

	// Get Ziti Edge Controller Port
	zitiEdgeCtrlPort, err := cmdhelper.GetZitiEdgeCtrlPort()
	handleVariableError(err, cmdhelper.ZitiEdgeCtrlPortVarName)

	// Get Ziti Edge Intermediate Name
	zitiEdgeIntermediateName, err := cmdhelper.GetZitiEdgeCtrlIntermediateName()
	handleVariableError(err, cmdhelper.ZitiEdgeCtrlPortVarName)

	// Get Ziti Edge Controller Hostname
	zitiEdgeCtrlHostname, err := cmdhelper.GetZitiEdgeCtrlHostname()
	handleVariableError(err, cmdhelper.ZitiEdgeCtrlHostnameVarName)

	options.ZitiHome = zitiHome
	options.Hostname = hostname
	options.ZitiPKI = zitiPKI
	options.ZitiFabCtrlPort = zitiFabCtrlPort
	options.ZitiCtrlIntermediateName = zitiCtrlIntName
	options.ZitiCtrlHostname = zitiCtrlHostname
	options.ZitiFabMgmtPort = zitiFabMgmtPort
	options.ZitiEdgeCtrlAPI = zitiEdgeCtrlAPI
	options.ZitiSigningIntermediateName = zitiSigningIntermediateName
	options.ZitiEdgeCtrlPort = zitiEdgeCtrlPort
	options.ZitiCtrlRawname = zitiCtrlRawname
	options.ZitiEdgeCtrlIntermediateName = zitiEdgeIntermediateName
	options.ZitiEdgeCtrlHostname = zitiEdgeCtrlHostname
}

func handleVariableError(err error, varName string) {
	if err != nil {
		logrus.Errorf("Unable to get %s", varName)
	}
}
