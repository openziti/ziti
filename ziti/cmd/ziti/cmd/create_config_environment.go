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
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

type EnvVariableMetaData struct {
	OS                                           string
	ZitiHomeVarName                              string
	ZitiCtrlNameVarName                          string
	ZitiCtrlPortVarName                          string
	ZitiEdgeRouterRawNameVarName                 string
	ZitiEdgeRouterPortVarName                    string
	ZitiEdgeCtrlIdentityCertVarName              string
	ZitiEdgeCtrlIdentityServerCertVarName        string
	ZitiEdgeCtrlIdentityKeyVarName               string
	ZitiEdgeCtrlIdentityCAVarName                string
	ZitiCtrlIdentityCertVarName                  string
	ZitiCtrlIdentityServerCertVarName            string
	ZitiCtrlIdentityKeyVarName                   string
	ZitiCtrlIdentityCAVarName                    string
	ZitiSigningCertVarName                       string
	ZitiSigningKeyVarName                        string
	ZitiCtrlMgmtListenerHostPortVarName          string
	ZitiEdgeCtrlListenerHostPortVarName          string
	ZitiEdgeCtrlAdvertisedHostPortVarName        string
	ZitiRouterIdentityCertVarName                string
	ZitiRouterIdentityServerCertVarName          string
	ZitiRouterIdentityKeyVarName                 string
	ZitiRouterIdentityCAVarName                  string
	ZitiCtrlListenerAddressVarName               string
	ZitiCtrlAdvertisedAddressVarName             string
	ZitiEdgeCtrlPortVarName                      string
	ZitiHomeVarDescription                       string
	ZitiCtrlNameVarDescription                   string
	ZitiEdgeRouterRawNameVarDescription          string
	ZitiEdgeRouterPortVarDescription             string
	ZitiEdgeCtrlIdentityCertVarDescription       string
	ZitiEdgeCtrlIdentityServerCertVarDescription string
	ZitiEdgeCtrlIdentityKeyVarDescription        string
	ZitiEdgeCtrlIdentityCAVarDescription         string
	ZitiCtrlIdentityCertVarDescription           string
	ZitiCtrlIdentityServerCertVarDescription     string
	ZitiCtrlIdentityKeyVarDescription            string
	ZitiCtrlIdentityCAVarDescription             string
	ZitiSigningCertVarDescription                string
	ZitiSigningKeyVarDescription                 string
	ZitiCtrlPortVarDescription                   string
	ZitiCtrlListenerAddressVarDescription        string
	ZitiCtrlAdvertisedAddressVarDescription      string
	ZitiCtrlMgmtListenerHostPortVarDescription   string
	ZitiEdgeCtrlListenerHostPortVarDescription   string
	ZitiEdgeCtrlAdvertisedHostPortVarDescription string
	ZitiRouterIdentityCertVarDescription         string
	ZitiRouterIdentityServerCertVarDescription   string
	ZitiRouterIdentityKeyVarDescription          string
	ZitiRouterIdentityCAVarDescription           string
	ZitiEdgeCtrlPortVarDescription               string
}

var (
	createConfigEnvironmentLong = templates.LongDesc(`
		Displays available environment variable manual overrides
`)

	createConfigEnvironmentExample = templates.Examples(`
		# Display environment variables and their values 
		ziti create config environment

		# Print an environment file to the console
		ziti create config environment --output stdout
	`)
)

//go:embed config_templates/environment.yml
var environmentConfigTemplate string

var environmentOptions *CreateConfigEnvironmentOptions

// CreateConfigEnvironmentOptions the options for the create environment command
type CreateConfigEnvironmentOptions struct {
	CreateConfigOptions
	*ConfigTemplateValues
	EnvVariableMetaData
	output string
}

// NewCmdCreateConfigEnvironment creates a command object for the "environment" command
func NewCmdCreateConfigEnvironment() *cobra.Command {

	environmentOptions = &CreateConfigEnvironmentOptions{
		ConfigTemplateValues: data,
		EnvVariableMetaData: EnvVariableMetaData{
			OS:                                           "Stuff",
			ZitiHomeVarName:                              constants.ZitiHomeVarName,
			ZitiHomeVarDescription:                       constants.ZitiHomeVarDescription,
			ZitiCtrlNameVarName:                          constants.ZitiCtrlNameVarName,
			ZitiCtrlNameVarDescription:                   constants.ZitiCtrlNameVarDescription,
			ZitiCtrlPortVarName:                          constants.ZitiCtrlPortVarName,
			ZitiCtrlPortVarDescription:                   constants.ZitiCtrlPortVarDescription,
			ZitiEdgeRouterRawNameVarName:                 constants.ZitiEdgeRouterRawNameVarName,
			ZitiEdgeRouterRawNameVarDescription:          constants.ZitiEdgeRouterRawNameVarDescription,
			ZitiEdgeRouterPortVarName:                    constants.ZitiEdgeRouterPortVarName,
			ZitiEdgeRouterPortVarDescription:             constants.ZitiEdgeRouterPortVarDescription,
			ZitiEdgeCtrlIdentityCertVarName:              constants.ZitiEdgeCtrlIdentityCertVarName,
			ZitiEdgeCtrlIdentityCertVarDescription:       constants.ZitiEdgeCtrlIdentityCertVarDescription,
			ZitiEdgeCtrlIdentityServerCertVarName:        constants.ZitiEdgeCtrlIdentityServerCertVarName,
			ZitiEdgeCtrlIdentityServerCertVarDescription: constants.ZitiEdgeCtrlIdentityServerCertVarDescription,
			ZitiEdgeCtrlIdentityKeyVarName:               constants.ZitiEdgeCtrlIdentityKeyVarName,
			ZitiEdgeCtrlIdentityKeyVarDescription:        constants.ZitiEdgeCtrlIdentityKeyVarDescription,
			ZitiEdgeCtrlIdentityCAVarName:                constants.ZitiEdgeCtrlIdentityCAVarName,
			ZitiEdgeCtrlIdentityCAVarDescription:         constants.ZitiEdgeCtrlIdentityCAVarDescription,
			ZitiCtrlIdentityCertVarName:                  constants.ZitiCtrlIdentityCertVarName,
			ZitiCtrlIdentityCertVarDescription:           constants.ZitiCtrlIdentityCertVarDescription,
			ZitiCtrlIdentityServerCertVarName:            constants.ZitiCtrlIdentityServerCertVarName,
			ZitiCtrlIdentityServerCertVarDescription:     constants.ZitiCtrlIdentityServerCertVarDescription,
			ZitiCtrlIdentityKeyVarName:                   constants.ZitiCtrlIdentityKeyVarName,
			ZitiCtrlIdentityKeyVarDescription:            constants.ZitiCtrlIdentityKeyVarDescription,
			ZitiCtrlIdentityCAVarName:                    constants.ZitiCtrlIdentityCAVarName,
			ZitiCtrlIdentityCAVarDescription:             constants.ZitiCtrlIdentityCAVarDescription,
			ZitiSigningCertVarName:                       constants.ZitiSigningCertVarName,
			ZitiSigningCertVarDescription:                constants.ZitiSigningCertVarDescription,
			ZitiSigningKeyVarName:                        constants.ZitiSigningKeyVarName,
			ZitiSigningKeyVarDescription:                 constants.ZitiSigningKeyVarDescription,
			ZitiRouterIdentityCertVarName:                constants.ZitiRouterIdentityCertVarName,
			ZitiRouterIdentityCertVarDescription:         constants.ZitiRouterIdentityCertVarDescription,
			ZitiRouterIdentityServerCertVarName:          constants.ZitiRouterIdentityServerCertVarName,
			ZitiRouterIdentityServerCertVarDescription:   constants.ZitiRouterIdentityServerCertVarDescription,
			ZitiRouterIdentityKeyVarName:                 constants.ZitiRouterIdentityKeyVarName,
			ZitiRouterIdentityKeyVarDescription:          constants.ZitiRouterIdentityKeyVarDescription,
			ZitiRouterIdentityCAVarName:                  constants.ZitiRouterIdentityCAVarName,
			ZitiRouterIdentityCAVarDescription:           constants.ZitiRouterIdentityCAVarDescription,
			ZitiCtrlListenerAddressVarName:               constants.ZitiCtrlListenerAddressVarName,
			ZitiCtrlListenerAddressVarDescription:        constants.ZitiCtrlListenerAddressVarDescription,
			ZitiCtrlAdvertisedAddressVarName:             constants.ZitiCtrlAdvertisedAddressVarName,
			ZitiCtrlAdvertisedAddressVarDescription:      constants.ZitiCtrlAdvertisedAddressVarDescription,
			ZitiEdgeCtrlListenerHostPortVarName:          constants.ZitiEdgeCtrlListenerHostPortVarName,
			ZitiEdgeCtrlListenerHostPortVarDescription:   constants.ZitiEdgeCtrlListenerHostPortVarDescription,
			ZitiEdgeCtrlAdvertisedHostPortVarName:        constants.ZitiEdgeCtrlAdvertisedHostPortVarName,
			ZitiEdgeCtrlAdvertisedHostPortVarDescription: constants.ZitiEdgeCtrlAdvertisedHostPortVarDescription,
			ZitiEdgeCtrlPortVarName:                      constants.ZitiEdgeCtrlPortVarName,
			ZitiEdgeCtrlPortVarDescription:               constants.ZitiEdgeCtrlPortVarDescription,
		},
	}

	cmd := &cobra.Command{
		Use:     "environment",
		Short:   "Display config environment variables",
		Aliases: []string{"env"},
		Long:    createConfigEnvironmentLong,
		Example: createConfigEnvironmentExample,
		PreRun: func(cmd *cobra.Command, args []string) {
			data.populateEnvVars()
			data.populateDefaults()
			SetZitiRouterIdentity(&data.Router, data.Router.Name)

			// Setup logging
			var logOut *os.File
			environmentOptions.OS = runtime.GOOS
			if environmentOptions.Verbose {
				logrus.SetLevel(logrus.DebugLevel)
				// Only print log to stdout if not printing config to stdout
				if strings.ToLower(environmentOptions.Output) != "stdout" {
					logOut = os.Stdout
				} else {
					logOut = os.Stderr
				}
				logrus.SetOutput(logOut)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			environmentOptions.Cmd = cmd
			environmentOptions.Args = args
			err := environmentOptions.run()
			cmdhelper.CheckErr(err)
		},
		PostRun: func(cmd *cobra.Command, args []string) {
			// Reset log output after run completes
			logrus.SetOutput(os.Stdout)
		},
	}

	createConfigLong := fmt.Sprintf("Creates a config file for specified Ziti component using environment variables which have default values but can be manually set to override the config output.\n\n"+
		"The following environment variables can be set to override config values (current value is displayed):\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s\n"+
		"%-36s %-50s %s",
		constants.ZitiHomeVarName, constants.ZitiHomeVarDescription, data.ZitiHome,
		constants.ZitiCtrlPortVarName, constants.ZitiCtrlPortVarDescription, data.Controller.Port,
		constants.ZitiCtrlNameVarName, constants.ZitiCtrlNameVarDescription, data.Controller.Name,
		constants.ZitiCtrlAdvertisedAddressVarName, constants.ZitiCtrlAdvertisedAddressVarDescription, data.Controller.AdvertisedAddress,
		constants.ZitiCtrlListenerAddressVarName, constants.ZitiCtrlListenerAddressVarDescription, data.Controller.ListenerAddress,
		constants.ZitiEdgeCtrlListenerHostPortVarName, constants.ZitiEdgeCtrlListenerHostPortVarDescription, data.Controller.Edge.ListenerHostPort,
		constants.ZitiEdgeCtrlAdvertisedHostPortVarName, constants.ZitiEdgeCtrlAdvertisedHostPortVarDescription, data.Controller.Edge.AdvertisedHostPort,
		constants.ZitiEdgeCtrlPortVarName, constants.ZitiEdgeCtrlPortVarDescription, data.Controller.Edge.Port,
		constants.ZitiEdgeRouterRawNameVarName, constants.ZitiEdgeRouterRawNameVarDescription, data.Router.Edge.Hostname,
		constants.ZitiEdgeRouterPortVarName, constants.ZitiEdgeRouterPortVarDescription, data.Router.Edge.Port,
		constants.ZitiRouterIdentityCertVarName, constants.ZitiRouterIdentityCertVarDescription, data.Router.IdentityCert,
		constants.ZitiRouterIdentityServerCertVarName, constants.ZitiRouterIdentityServerCertVarDescription, data.Router.IdentityServerCert,
		constants.ZitiRouterIdentityKeyVarName, constants.ZitiRouterIdentityKeyVarDescription, data.Router.IdentityKey,
		constants.ZitiRouterIdentityCAVarName, constants.ZitiRouterIdentityCAVarDescription, data.Router.IdentityCA,
		constants.ZitiCtrlIdentityCertVarName, constants.ZitiCtrlIdentityCertVarDescription, data.Controller.IdentityCert,
		constants.ZitiCtrlIdentityServerCertVarName, constants.ZitiCtrlIdentityServerCertVarDescription, data.Controller.IdentityServerCert,
		constants.ZitiCtrlIdentityKeyVarName, constants.ZitiCtrlIdentityKeyVarDescription, data.Controller.IdentityKey,
		constants.ZitiCtrlIdentityCAVarName, constants.ZitiCtrlIdentityCAVarDescription, data.Controller.IdentityCA,
		constants.ZitiEdgeCtrlIdentityCertVarName, constants.ZitiEdgeCtrlIdentityCertVarDescription, data.Controller.Edge.IdentityCert,
		constants.ZitiEdgeCtrlIdentityServerCertVarName, constants.ZitiEdgeCtrlIdentityServerCertVarDescription, data.Controller.Edge.IdentityServerCert,
		constants.ZitiEdgeCtrlIdentityKeyVarName, constants.ZitiEdgeCtrlIdentityKeyVarDescription, data.Controller.Edge.IdentityKey,
		constants.ZitiEdgeCtrlIdentityCAVarName, constants.ZitiEdgeCtrlIdentityCAVarDescription, data.Controller.Edge.IdentityCA,
		constants.ZitiSigningCertVarName, constants.ZitiSigningCertVarDescription, data.Controller.Edge.ZitiSigningCert,
		constants.ZitiSigningKeyVarName, constants.ZitiSigningKeyVarDescription, data.Controller.Edge.ZitiSigningKey)

	cmd.Long = createConfigLong

	environmentOptions.addCreateFlags(cmd)

	return cmd
}

// run implements the command
func (options *CreateConfigEnvironmentOptions) run() error {

	tmpl, err := template.New("environment-config").Parse(environmentConfigTemplate)
	if err != nil {
		return err
	}

	var f *os.File
	if strings.ToLower(options.Output) != "stdout" {
		// Check if the path exists, fail if it doesn't
		basePath := filepath.Dir(options.Output) + "/"
		if _, err := os.Stat(filepath.Dir(basePath)); os.IsNotExist(err) {
			logrus.Fatalf("Provided path: [%s] does not exist\n", basePath)
			return err
		}

		f, err = os.Create(options.Output)
		logrus.Debugf("Created output file: %s", options.Output)
		if err != nil {
			return errors.Wrapf(err, "unable to create config file: %s", options.Output)
		}
	} else {
		f = os.Stdout
	}
	defer func() { _ = f.Close() }()

	if err := tmpl.Execute(f, options); err != nil {
		return errors.Wrap(err, "unable to execute template")
	}

	logrus.Debugf("Environment configuration file generated successfully and written to: %s", options.Output)

	return nil
}
