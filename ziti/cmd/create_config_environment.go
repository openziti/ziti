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
	_ "embed"
	"fmt"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	"github.com/openziti/ziti/ziti/constants"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type EnvVariableTemplateData struct {
	OSCommentPrefix string
	OSVarDeclare    string
	EnvVars         []EnvVar
}

type EnvVar struct {
	Name        string
	Description string
	Value       string
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
	EnvVariableTemplateData
	output string
}

// NewCmdCreateConfigEnvironment creates a command object for the "environment" command
func NewCmdCreateConfigEnvironment() *cobra.Command {

	environmentOptions = &CreateConfigEnvironmentOptions{}

	cmd := &cobra.Command{
		Use:     "environment",
		Short:   "Display config environment variables",
		Aliases: []string{"env"},
		Long:    createConfigEnvironmentLong,
		Example: createConfigEnvironmentExample,
		PreRun: func(cmd *cobra.Command, args []string) {
			data.populateEnvVars()
			data.populateDefaults()
			// Set router identities
			SetZitiRouterIdentity(&data.Router, validateRouterName(""))
			// Set up other identity info
			SetControllerIdentity(&data.Controller)
			SetEdgeConfig(&data.Controller)
			SetWebConfig(&data.Controller)

			environmentOptions.EnvVars = []EnvVar{
				{constants.ZitiHomeVarName, constants.ZitiHomeVarDescription, data.ZitiHome},
				{constants.ZitiCtrlNameVarName, constants.ZitiCtrlNameVarDescription, data.Controller.Name},
				{constants.ZitiCtrlPortVarName, constants.ZitiCtrlPortVarDescription, data.Controller.Port},
				{constants.ZitiEdgeRouterRawNameVarName, constants.ZitiEdgeRouterRawNameVarDescription, data.Router.Edge.Hostname},
				{constants.ZitiEdgeRouterPortVarName, constants.ZitiEdgeRouterPortVarDescription, data.Router.Edge.Port},
				{constants.ZitiEdgeRouterListenerBindPortVarName, constants.ZitiEdgeRouterListenerBindPortVarDescription, data.Router.Edge.ListenerBindPort},
				{constants.ZitiEdgeCtrlIdentityCertVarName, constants.ZitiEdgeCtrlIdentityCertVarDescription, data.Controller.Edge.IdentityCert},
				{constants.ZitiEdgeCtrlIdentityServerCertVarName, constants.ZitiEdgeCtrlIdentityServerCertVarDescription, data.Controller.Edge.IdentityServerCert},
				{constants.ZitiEdgeCtrlIdentityKeyVarName, constants.ZitiEdgeCtrlIdentityKeyVarDescription, data.Controller.Edge.IdentityKey},
				{constants.ZitiEdgeCtrlIdentityCAVarName, constants.ZitiEdgeCtrlIdentityCAVarDescription, data.Controller.Edge.IdentityCA},
				{constants.ZitiCtrlIdentityCertVarName, constants.ZitiCtrlIdentityCertVarDescription, data.Controller.IdentityCert},
				{constants.ZitiCtrlIdentityServerCertVarName, constants.ZitiCtrlIdentityServerCertVarDescription, data.Controller.IdentityServerCert},
				{constants.ZitiCtrlIdentityKeyVarName, constants.ZitiCtrlIdentityKeyVarDescription, data.Controller.IdentityKey},
				{constants.ZitiCtrlIdentityCAVarName, constants.ZitiCtrlIdentityCAVarDescription, data.Controller.IdentityCA},
				{constants.ZitiSigningCertVarName, constants.ZitiSigningCertVarDescription, data.Controller.Edge.ZitiSigningCert},
				{constants.ZitiSigningKeyVarName, constants.ZitiSigningKeyVarDescription, data.Controller.Edge.ZitiSigningKey},
				{constants.ZitiRouterIdentityCertVarName, constants.ZitiRouterIdentityCertVarDescription, data.Router.IdentityCert},
				{constants.ZitiRouterIdentityServerCertVarName, constants.ZitiRouterIdentityServerCertVarDescription, data.Router.IdentityServerCert},
				{constants.ZitiRouterIdentityKeyVarName, constants.ZitiRouterIdentityKeyVarDescription, data.Router.IdentityKey},
				{constants.ZitiRouterIdentityCAVarName, constants.ZitiRouterIdentityCAVarDescription, data.Router.IdentityCA},
				{constants.ZitiEdgeRouterIPOverrideVarName, constants.ZitiEdgeRouterIPOverrideVarDescription, data.Router.Edge.IPOverride},
				{constants.ZitiEdgeRouterAdvertisedHostVarName, constants.ZitiEdgeRouterAdvertisedHostVarDescription, data.Router.Edge.AdvertisedHost},
				{constants.ZitiCtrlListenerAddressVarName, constants.ZitiCtrlListenerAddressVarDescription, data.Controller.ListenerAddress},
				{constants.ZitiCtrlAdvertisedAddressVarName, constants.ZitiCtrlAdvertisedAddressVarDescription, data.Controller.AdvertisedAddress},
				{constants.ZitiEdgeCtrlListenerHostPortVarName, constants.ZitiEdgeCtrlListenerHostPortVarDescription, data.Controller.Edge.ListenerHostPort},
				{constants.ZitiEdgeCtrlAdvertisedHostPortVarName, constants.ZitiEdgeCtrlAdvertisedHostPortVarDescription, data.Controller.Edge.AdvertisedHostPort},
				{constants.ZitiEdgeCtrlAdvertisedPortVarName, constants.ZitiEdgeCtrlAdvertisedPortVarDescription, data.Controller.Edge.AdvertisedPort},
				{constants.ZitiEdgeIdentityEnrollmentDurationVarName, constants.ZitiEdgeIdentityEnrollmentDurationVarDescription, data.Controller.EdgeIdentityDuration.String()},
				{constants.ZitiEdgeRouterEnrollmentDurationVarName, constants.ZitiEdgeRouterEnrollmentDurationVarDescription, data.Controller.EdgeRouterDuration.String()},
			}

			// Setup logging
			var logOut *os.File
			// Figure out the correct comment prefix and variable declaration command
			if runtime.GOOS == "windows" {
				environmentOptions.OSCommentPrefix = "rem"
				environmentOptions.OSVarDeclare = "SET"
			} else {
				environmentOptions.OSCommentPrefix = "#"
				environmentOptions.OSVarDeclare = "export"
			}
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
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s\n"+
		"%-40s %-50s %s",
		constants.ZitiHomeVarName, constants.ZitiHomeVarDescription, data.ZitiHome,
		constants.ZitiCtrlPortVarName, constants.ZitiCtrlPortVarDescription, data.Controller.Port,
		constants.ZitiCtrlNameVarName, constants.ZitiCtrlNameVarDescription, data.Controller.Name,
		constants.ZitiCtrlAdvertisedAddressVarName, constants.ZitiCtrlAdvertisedAddressVarDescription, data.Controller.AdvertisedAddress,
		constants.ZitiCtrlListenerAddressVarName, constants.ZitiCtrlListenerAddressVarDescription, data.Controller.ListenerAddress,
		constants.ZitiEdgeCtrlListenerHostPortVarName, constants.ZitiEdgeCtrlListenerHostPortVarDescription, data.Controller.Edge.ListenerHostPort,
		constants.ZitiEdgeCtrlAdvertisedHostPortVarName, constants.ZitiEdgeCtrlAdvertisedHostPortVarDescription, data.Controller.Edge.AdvertisedHostPort,
		constants.ZitiEdgeCtrlAdvertisedPortVarName, constants.ZitiEdgeCtrlAdvertisedPortVarDescription, data.Controller.Edge.AdvertisedPort,
		constants.ZitiEdgeRouterRawNameVarName, constants.ZitiEdgeRouterRawNameVarDescription, data.Router.Edge.Hostname,
		constants.ZitiEdgeRouterPortVarName, constants.ZitiEdgeRouterPortVarDescription, data.Router.Edge.Port,
		constants.ZitiEdgeRouterListenerBindPortVarName, constants.ZitiEdgeRouterListenerBindPortVarDescription, data.Router.Edge.ListenerBindPort,
		constants.ZitiRouterIdentityCertVarName, constants.ZitiRouterIdentityCertVarDescription, data.Router.IdentityCert,
		constants.ZitiRouterIdentityServerCertVarName, constants.ZitiRouterIdentityServerCertVarDescription, data.Router.IdentityServerCert,
		constants.ZitiRouterIdentityKeyVarName, constants.ZitiRouterIdentityKeyVarDescription, data.Router.IdentityKey,
		constants.ZitiRouterIdentityCAVarName, constants.ZitiRouterIdentityCAVarDescription, data.Router.IdentityCA,
		constants.ZitiEdgeRouterIPOverrideVarName, constants.ZitiEdgeRouterIPOverrideVarDescription, data.Router.Edge.IPOverride,
		constants.ZitiEdgeRouterAdvertisedHostVarName, constants.ZitiEdgeRouterAdvertisedHostVarDescription, data.Router.Edge.AdvertisedHost,
		constants.ZitiCtrlIdentityCertVarName, constants.ZitiCtrlIdentityCertVarDescription, data.Controller.IdentityCert,
		constants.ZitiCtrlIdentityServerCertVarName, constants.ZitiCtrlIdentityServerCertVarDescription, data.Controller.IdentityServerCert,
		constants.ZitiCtrlIdentityKeyVarName, constants.ZitiCtrlIdentityKeyVarDescription, data.Controller.IdentityKey,
		constants.ZitiCtrlIdentityCAVarName, constants.ZitiCtrlIdentityCAVarDescription, data.Controller.IdentityCA,
		constants.ZitiEdgeCtrlIdentityCertVarName, constants.ZitiEdgeCtrlIdentityCertVarDescription, data.Controller.Edge.IdentityCert,
		constants.ZitiEdgeCtrlIdentityServerCertVarName, constants.ZitiEdgeCtrlIdentityServerCertVarDescription, data.Controller.Edge.IdentityServerCert,
		constants.ZitiEdgeCtrlIdentityKeyVarName, constants.ZitiEdgeCtrlIdentityKeyVarDescription, data.Controller.Edge.IdentityKey,
		constants.ZitiEdgeCtrlIdentityCAVarName, constants.ZitiEdgeCtrlIdentityCAVarDescription, data.Controller.Edge.IdentityCA,
		constants.ZitiSigningCertVarName, constants.ZitiSigningCertVarDescription, data.Controller.Edge.ZitiSigningCert,
		constants.ZitiSigningKeyVarName, constants.ZitiSigningKeyVarDescription, data.Controller.Edge.ZitiSigningKey,
		constants.ZitiEdgeIdentityEnrollmentDurationVarName, constants.ZitiEdgeIdentityEnrollmentDurationVarDescription, fmt.Sprintf("%.0f", data.Controller.EdgeIdentityDuration.Minutes()),
		constants.ZitiEdgeRouterEnrollmentDurationVarName, constants.ZitiEdgeRouterEnrollmentDurationVarDescription, fmt.Sprintf("%.0f", data.Controller.EdgeRouterDuration.Minutes()))

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
