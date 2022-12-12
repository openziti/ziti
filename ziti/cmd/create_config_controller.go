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
	edge "github.com/openziti/edge/controller/config"
	helpers2 "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

const (
	optionCtrlPort                       = "ctrlPort"
	optionDatabaseFile                   = "databaseFile"
	optionEdgeIdentityEnrollmentDuration = "identityEnrollmentDuration"
	optionEdgeRouterEnrollmentDuration   = "routerEnrollmentDuration"
)

var (
	createConfigControllerLong = templates.LongDesc(`
		Creates the controller config
`)

	createConfigControllerExample = templates.Examples(`
		# Create the controller config 
		ziti create config controller

		# Create the controller config with a particular ctrlListener host and port
		ziti create config controller --ctrlPort 6262

		# Print the controller config to the console
		ziti create config controller --output stdout

		# Print the controller config to a file
		ziti create config controller --output <path to file>/<filename>.yaml

		# Create the controller config with an edge router enrollment duration of 2 hours
		ziti create config controller --identityEnrollmentDuration 2h
		# OR
		ziti create config controller --identityEnrollmentDuration 120m
	`)
)

//go:embed config_templates/controller.yml
var controllerConfigTemplate string

// CreateConfigControllerOptions the options for the create spring command
type CreateConfigControllerOptions struct {
	CreateConfigOptions

	CtrlPort                       string
	MgmtListener                   string
	EdgeIdentityEnrollmentDuration time.Duration
	EdgeRouterEnrollmentDuration   time.Duration
}

// NewCmdCreateConfigController creates a command object for the "create" command
func NewCmdCreateConfigController() *cobra.Command {
	controllerOptions := &CreateConfigControllerOptions{}

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

			data.populateEnvVars()
			data.populateDefaults()

			// Update controller specific values with configOptions passed in if the argument was provided or the value is currently blank
			if data.Controller.Port == "" || controllerOptions.CtrlPort != constants.DefaultZitiControllerPort {
				data.Controller.Port = controllerOptions.CtrlPort
			}
			// Update with the passed in arg if it's not the default (CLI flag should override other methods of modifying these values)
			if controllerOptions.EdgeIdentityEnrollmentDuration != edge.DefaultEdgeEnrollmentDuration {
				data.Controller.EdgeIdentityDuration = controllerOptions.EdgeIdentityEnrollmentDuration
			}
			if controllerOptions.EdgeRouterEnrollmentDuration != edge.DefaultEdgeEnrollmentDuration {
				data.Controller.EdgeRouterDuration = controllerOptions.EdgeRouterEnrollmentDuration
			}

			// process identity information
			SetControllerIdentity(&data.Controller)
			SetEdgeConfig(&data.Controller)
			SetWebConfig(&data.Controller)

		},
		Run: func(cmd *cobra.Command, args []string) {
			controllerOptions.Cmd = cmd
			controllerOptions.Args = args
			err := controllerOptions.run(data)
			helpers2.CheckErr(err)
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
	cmd.Flags().StringVar(&options.CtrlPort, optionCtrlPort, constants.DefaultZitiControllerPort, "port used for the router to controller communication")
	cmd.Flags().StringVar(&options.DatabaseFile, optionDatabaseFile, "ctrl.db", "location of the database file")
	cmd.Flags().DurationVar(&options.EdgeIdentityEnrollmentDuration, optionEdgeIdentityEnrollmentDuration, edge.DefaultEdgeEnrollmentDuration, "the edge identity enrollment duration, use 0h0m0s format")
	cmd.Flags().DurationVar(&options.EdgeRouterEnrollmentDuration, optionEdgeRouterEnrollmentDuration, edge.DefaultEdgeEnrollmentDuration, "the edge router enrollment duration, use 0h0m0s format")
}

// run implements the command
func (options *CreateConfigControllerOptions) run(data *ConfigTemplateValues) error {

	tmpl, err := template.New("controller-config").Parse(controllerConfigTemplate)
	if err != nil {
		return err
	}

	var f *os.File
	if strings.ToLower(options.Output) != "stdout" {
		// Check if the path exists, fail if it doesn't
		basePath := filepath.Dir(options.Output) + "/"
		if _, err := os.Stat(filepath.Dir(basePath)); os.IsNotExist(err) {
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

	if err := tmpl.Execute(f, data); err != nil {
		return errors.Wrap(err, "unable to execute template")
	}

	logrus.Debugf("Controller configuration generated successfully and written to: %s", options.Output)

	return nil
}

func hostnameOrNetworkName() string {
	val := os.Getenv("ZITI_NETWORK_NAME")
	if val == "" {
		h, err := os.Hostname()
		if err != nil {
			return "localhost"
		}
		return h
	}
	return val
}

func SetControllerIdentity(data *ControllerTemplateValues) {
	SetControllerIdentityCert(data)
	SetControllerIdentityServerCert(data)
	SetControllerIdentityKey(data)
	SetControllerIdentityCA(data)
}
func SetControllerIdentityCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiCtrlIdentityCertVarName)
	if val == "" {
		val = workingDir + "/" + hostnameOrNetworkName() + ".cert" // default
	}
	c.IdentityCert = helpers2.NormalizePath(val)
}
func SetControllerIdentityServerCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiCtrlIdentityServerCertVarName)
	if val == "" {
		val = workingDir + "/" + hostnameOrNetworkName() + ".server.chain.cert" // default
	}
	c.IdentityServerCert = helpers2.NormalizePath(val)
}
func SetControllerIdentityKey(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiCtrlIdentityKeyVarName)
	if val == "" {
		val = workingDir + "/" + hostnameOrNetworkName() + ".key" // default
	}
	c.IdentityKey = helpers2.NormalizePath(val)
}
func SetControllerIdentityCA(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiCtrlIdentityCAVarName)
	if val == "" {
		val = workingDir + "/" + hostnameOrNetworkName() + ".ca" // default
	}
	c.IdentityCA = helpers2.NormalizePath(val)
}

func SetEdgeConfig(data *ControllerTemplateValues) {
	SetEdgeSigningCert(data)
	SetEdgeSigningKey(data)
}
func SetEdgeSigningCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiSigningCertVarName)
	if val == "" {
		val = workingDir + "/" + hostnameOrNetworkName() + ".signing.cert" // default
	}
	c.Edge.ZitiSigningCert = helpers2.NormalizePath(val)
}
func SetEdgeSigningKey(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiSigningKeyVarName)
	if val == "" {
		val = workingDir + "/" + hostnameOrNetworkName() + ".signing.key" // default
	}
	c.Edge.ZitiSigningKey = helpers2.NormalizePath(val)
}

func SetWebConfig(data *ControllerTemplateValues) {
	SetWebIdentityCert(data)
	SetWebIdentityServerCert(data)
	SetWebIdentityKey(data)
	SetWebIdentityCA(data)
}
func SetWebIdentityCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiEdgeCtrlIdentityCertVarName)
	if val == "" {
		val = c.IdentityCert //default
	}
	c.Edge.IdentityCert = helpers2.NormalizePath(val)
}
func SetWebIdentityServerCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiEdgeCtrlIdentityServerCertVarName)
	if val == "" {
		val = c.IdentityServerCert //default
	}
	c.Edge.IdentityServerCert = helpers2.NormalizePath(val)
}
func SetWebIdentityKey(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiEdgeCtrlIdentityKeyVarName)
	if val == "" {
		val = c.IdentityKey //default
	}
	c.Edge.IdentityKey = helpers2.NormalizePath(val)
}
func SetWebIdentityCA(c *ControllerTemplateValues) {
	val := os.Getenv(constants.ZitiEdgeCtrlIdentityCAVarName)
	if val == "" {
		val = c.IdentityCA //default
	}
	c.Edge.IdentityCA = helpers2.NormalizePath(val)
}
