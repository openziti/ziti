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

package create

import (
	_ "embed"
	edge "github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/zac"
	"github.com/openziti/ziti/ziti/cmd/helpers"
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
	optionEdgeIdentityEnrollmentDuration = "identityEnrollmentDuration"
	optionEdgeRouterEnrollmentDuration   = "routerEnrollmentDuration"
	optionMinCluster                     = "minCluster"
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
	EdgeIdentityEnrollmentDuration time.Duration
	EdgeRouterEnrollmentDuration   time.Duration
	MinCluster                     int
	Console                        ConsoleOptions
}

type ConsoleOptions struct {
	Disabled  bool
	Location string
}

type CreateControllerConfigCmd struct {
	*cobra.Command
	ConfigData *ConfigTemplateValues
}

// NewCmdCreateConfigController creates a command object for the "create" command
func NewCmdCreateConfigController() *CreateControllerConfigCmd {
	controllerOptions := &CreateConfigControllerOptions{}
	data := &ConfigTemplateValues{}
	cmd := &CreateControllerConfigCmd{
		ConfigData: data,
		Command: &cobra.Command{
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

				data.PopulateConfigValues()
				data.Controller.Ctrl.MinClusterSize = controllerOptions.MinCluster

				// Update controller specific values with configOptions passed in if the argument was provided or the value is currently blank
				if data.Controller.Ctrl.AdvertisedPort == "" || controllerOptions.CtrlPort != constants.DefaultCtrlAdvertisedPort {
					data.Controller.Ctrl.AdvertisedPort = controllerOptions.CtrlPort
				}
				// Update with the passed in arg if it's not the default (CLI flag should override other methods of modifying these values)
				if controllerOptions.EdgeIdentityEnrollmentDuration != edge.DefaultEdgeEnrollmentDuration {
					data.Controller.EdgeEnrollment.EdgeIdentityDuration = controllerOptions.EdgeIdentityEnrollmentDuration
				}
				if controllerOptions.EdgeRouterEnrollmentDuration != edge.DefaultEdgeEnrollmentDuration {
					data.Controller.EdgeEnrollment.EdgeRouterDuration = controllerOptions.EdgeRouterEnrollmentDuration
				}

				// process identity information
				SetControllerIdentity(&data.Controller)
				SetEdgeConfig(&data.Controller)
				SetWebConfig(&data.Controller)
				// process console options
				SetConsoleConfig(&data.Controller.Web.BindPoints.Console, &controllerOptions.Console)

			},
			Run: func(cmd *cobra.Command, args []string) {
				controllerOptions.Cmd = cmd
				controllerOptions.Args = args
				err := controllerOptions.run(data)
				helpers.CheckErr(err)
			},
			PostRun: func(cmd *cobra.Command, args []string) {
				// Reset log output after run completes
				logrus.SetOutput(os.Stdout)
			},
		},
	}
	controllerOptions.addCreateFlags(cmd.Command)
	controllerOptions.addFlags(cmd.Command)

	return cmd
}

func (options *CreateConfigControllerOptions) addFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&options.CtrlPort, optionCtrlPort, constants.DefaultCtrlAdvertisedPort, "port used for the router to controller communication")
	cmd.Flags().DurationVar(&options.EdgeIdentityEnrollmentDuration, optionEdgeIdentityEnrollmentDuration, edge.DefaultEdgeEnrollmentDuration, "the edge identity enrollment duration, use 0h0m0s format")
	cmd.Flags().DurationVar(&options.EdgeRouterEnrollmentDuration, optionEdgeRouterEnrollmentDuration, edge.DefaultEdgeEnrollmentDuration, "the edge router enrollment duration, use 0h0m0s format")
	cmd.Flags().IntVar(&options.MinCluster, optionMinCluster, 0, "minimum cluster size. Enables HA mode if > 0")
	cmd.Flags().BoolVar(&options.Console.Disabled, "no-console", false, "disable console")
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

		//only close things we open
		defer func() { _ = f.Close() }()
	} else {
		f = os.Stdout
	}

	if err := tmpl.Execute(f, data); err != nil {
		return errors.Wrap(err, "unable to execute template")
	}

	logrus.Debugf("Controller configuration generated successfully and written to: %s", options.Output)

	return nil
}

func SetControllerIdentity(data *ControllerTemplateValues) {
	SetControllerIdentityCert(data)
	SetControllerIdentityServerCert(data)
	SetControllerIdentityKey(data)
	SetControllerIdentityCA(data)
}

func SetControllerIdentityCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.PkiCtrlCertVarName)
	if val == "" {
		val = helpers.GetZitiHome() + "/" + helpers.HostnameOrNetworkName() + ".cert" // default
	}
	c.Identity.Cert = helpers.NormalizePath(val)
}

func SetControllerIdentityServerCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.PkiCtrlServerCertVarName)
	if val == "" {
		val = helpers.GetZitiHome() + "/" + helpers.HostnameOrNetworkName() + ".server.chain.cert" // default
	}
	c.Identity.ServerCert = helpers.NormalizePath(val)
}

func SetControllerIdentityKey(c *ControllerTemplateValues) {
	val := os.Getenv(constants.PkiCtrlKeyVarName)
	if val == "" {
		val = helpers.GetZitiHome() + "/" + helpers.HostnameOrNetworkName() + ".key" // default
	}
	c.Identity.Key = helpers.NormalizePath(val)
}

func SetControllerIdentityCA(c *ControllerTemplateValues) {
	val := os.Getenv(constants.PkiCtrlCAVarName)
	if val == "" {
		val = helpers.GetZitiHome() + "/" + helpers.HostnameOrNetworkName() + ".ca" // default
	}
	c.Identity.Ca = helpers.NormalizePath(val)
}

func SetEdgeConfig(data *ControllerTemplateValues) {
	SetEdgeSigningCert(data)
	SetEdgeSigningKey(data)
}

func SetEdgeSigningCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.PkiSignerCertVarName)
	if val == "" {
		val = helpers.GetZitiHome() + "/" + helpers.HostnameOrNetworkName() + ".signing.cert" // default
	}
	c.EdgeEnrollment.SigningCert = helpers.NormalizePath(val)

}

func SetEdgeSigningKey(c *ControllerTemplateValues) {
	val := os.Getenv(constants.PkiSignerKeyVarName)
	if val == "" {
		val = helpers.GetZitiHome() + "/" + helpers.HostnameOrNetworkName() + ".signing.key" // default
	}
	c.EdgeEnrollment.SigningCertKey = helpers.NormalizePath(val)
}

func SetWebConfig(data *ControllerTemplateValues) {
	SetWebIdentityCert(data)
	SetWebIdentityServerCert(data)
	SetWebIdentityKey(data)
	SetWebIdentityCA(data)
	SetCtrlAltServerCerts(data)
}

func SetConsoleConfig(v *ConsoleValues, o *ConsoleOptions) {
	v.Disabled = o.Disabled
	location := os.Getenv(constants.CtrlConsoleLocationVarName)
	if location == "" {
		v.Location = zac.DefaultLocation
	} else {
		v.Location = helpers.NormalizePath(location)
	}
}

func SetWebIdentityCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.CtrlPkiEdgeCertVarName)
	if val == "" {
		val = c.Identity.Cert //default
	}
	c.Web.Identity.Cert = helpers.NormalizePath(val)
}

func SetWebIdentityServerCert(c *ControllerTemplateValues) {
	val := os.Getenv(constants.CtrlPkiEdgeServerCertVarName)
	if val == "" {
		val = c.Identity.ServerCert //default
	}
	c.Web.Identity.ServerCert = helpers.NormalizePath(val)
}

func SetWebIdentityKey(c *ControllerTemplateValues) {
	val := os.Getenv(constants.CtrlPkiEdgeKeyVarName)
	if val == "" {
		val = c.Identity.Key //default
	}
	c.Web.Identity.Key = helpers.NormalizePath(val)
}

func SetWebIdentityCA(c *ControllerTemplateValues) {
	val := os.Getenv(constants.CtrlPkiEdgeCAVarName)
	if val == "" {
		val = c.Identity.Ca //default
	}
	c.Web.Identity.Ca = helpers.NormalizePath(val)
}

func SetCtrlAltServerCerts(c *ControllerTemplateValues) {
	c.Web.Identity.AltCertsEnabled = false
	altServerCert := os.Getenv(constants.PkiAltServerCertVarName)
	if altServerCert == "" {
		return //exit unless both vars are set
	}
	altServerKey := os.Getenv(constants.PkiAltServerKeyVarName)
	if altServerKey == "" {
		return //exit unless both vars are set
	}
	c.Web.Identity.AltCertsEnabled = true
	c.Web.Identity.AltServerCert = helpers.NormalizePath(altServerCert)
	c.Web.Identity.AltServerKey = helpers.NormalizePath(altServerKey)
}
