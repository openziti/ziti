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
	"fmt"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	"github.com/openziti/ziti/ziti/constants"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
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
}

// NewCmdCreateConfigEnvironment creates a command object for the "environment" command
func NewCmdCreateConfigEnvironment() *cobra.Command {
	environmentOptions = &CreateConfigEnvironmentOptions{}
	data := &ConfigTemplateValues{}

	cmd := &cobra.Command{
		Use:     "environment",
		Short:   "Display config environment variables",
		Aliases: []string{"env"},
		Long:    createConfigEnvironmentLong,
		Example: createConfigEnvironmentExample,
		PreRun: func(cmd *cobra.Command, args []string) {
			data.PopulateConfigValues()
			// Set router identities
			SetZitiRouterIdentity(&data.Router, validateRouterName(""))
			// Set up other identity info
			SetControllerIdentity(&data.Controller)
			SetEdgeConfig(&data.Controller)
			SetWebConfig(&data.Controller)

			environmentOptions.EnvVars = []EnvVar{
				{constants.ZitiHomeVarName, constants.ZitiHomeVarDescription, data.ZitiHome},
				{constants.PkiCtrlCertVarName, constants.PkiCtrlCertVarDescription, data.Controller.Identity.Cert},
				{constants.PkiCtrlServerCertVarName, constants.PkiCtrlServerCertVarDescription, data.Controller.Identity.ServerCert},
				{constants.PkiCtrlKeyVarName, constants.PkiCtrlKeyVarDescription, data.Controller.Identity.Key},
				{constants.PkiCtrlCAVarName, constants.PkiCtrlCAVarDescription, data.Controller.Identity.Ca},
				{constants.CtrlBindAddressVarName, constants.CtrlBindAddressVarDescription, data.Controller.Ctrl.BindAddress},
				{constants.CtrlAdvertisedAddressVarName, constants.CtrlAdvertisedAddressVarDescription, data.Controller.Ctrl.AdvertisedAddress},
				{constants.CtrlAdvertisedPortVarName, constants.CtrlAdvertisedPortVarDescription, data.Controller.Ctrl.AdvertisedPort},
				{constants.CtrlEdgeAdvertisedAddressVarName, constants.CtrlEdgeAdvertisedAddressVarDescription, data.Controller.EdgeApi.Address},
				{constants.CtrlEdgeAltAdvertisedAddressVarName, constants.CtrlEdgeAltAdvertisedAddressVarDescription, data.Controller.EdgeApi.Address},
				{constants.CtrlEdgeAdvertisedPortVarName, constants.CtrlEdgeAdvertisedPortVarDescription, data.Controller.EdgeApi.Port},
				{constants.CtrlEdgeBindAddressVarName, constants.CtrlEdgeBindAddressVarDescription, data.Controller.EdgeApi.Address},
				{constants.PkiSignerCertVarName, constants.PkiSignerCertVarDescription, data.Controller.EdgeEnrollment.SigningCert},
				{constants.PkiSignerKeyVarName, constants.PkiSignerKeyVarDescription, data.Controller.EdgeEnrollment.SigningCertKey},
				{constants.CtrlEdgeIdentityEnrollmentDurationVarName, constants.CtrlEdgeIdentityEnrollmentDurationVarDescription, strconv.FormatInt(int64(data.Controller.EdgeEnrollment.EdgeIdentityDuration), 10)},
				{constants.CtrlEdgeRouterEnrollmentDurationVarName, constants.CtrlEdgeRouterEnrollmentDurationVarDescription, strconv.FormatInt(int64(data.Controller.EdgeEnrollment.EdgeRouterDuration), 10)},
				{constants.CtrlEdgeAdvertisedAddressVarName, constants.CtrlEdgeAdvertisedAddressVarDescription, data.Controller.Web.BindPoints.AddressAddress},
				{constants.CtrlEdgeAdvertisedPortVarName, constants.CtrlEdgeAdvertisedPortVarDescription, data.Controller.Web.BindPoints.AddressPort},
				{constants.CtrlPkiEdgeCertVarName, constants.CtrlPkiEdgeCertVarDescription, data.Controller.Web.Identity.Cert},
				{constants.CtrlPkiEdgeServerCertVarName, constants.CtrlPkiEdgeServerCertVarDescription, data.Controller.Web.Identity.ServerCert},
				{constants.CtrlPkiEdgeKeyVarName, constants.CtrlPkiEdgeKeyVarDescription, data.Controller.Web.Identity.Key},
				{constants.CtrlPkiEdgeCAVarName, constants.CtrlPkiEdgeCAVarDescription, data.Controller.Web.Identity.Ca},
				{constants.PkiAltServerCertVarName, constants.PkiAltServerCertVarDescription, data.Controller.Web.Identity.AltServerCert},
				{constants.PkiAltServerKeyVarName, constants.PkiAltServerKeyVarDescription, data.Controller.Web.Identity.AltServerKey},
				{constants.ZitiEdgeRouterNameVarName, constants.ZitiEdgeRouterNameVarDescription, data.Router.Name},
				{constants.ZitiEdgeRouterPortVarName, constants.ZitiEdgeRouterPortVarDescription, data.Router.Edge.Port},
				{constants.ZitiEdgeRouterListenerBindPortVarName, constants.ZitiEdgeRouterListenerBindPortVarDescription, data.Router.Edge.ListenerBindPort},
				{constants.ZitiRouterIdentityCertVarName, constants.ZitiRouterIdentityCertVarDescription, data.Router.IdentityCert},
				{constants.ZitiRouterIdentityServerCertVarName, constants.ZitiRouterIdentityServerCertVarDescription, data.Router.IdentityServerCert},
				{constants.ZitiRouterIdentityKeyVarName, constants.ZitiRouterIdentityKeyVarDescription, data.Router.IdentityKey},
				{constants.ZitiRouterIdentityCAVarName, constants.ZitiRouterIdentityCAVarDescription, data.Router.IdentityCA},
				{constants.ZitiEdgeRouterIPOverrideVarName, constants.ZitiEdgeRouterIPOverrideVarDescription, data.Router.Edge.IPOverride},
				{constants.ZitiEdgeRouterAdvertisedAddressVarName, constants.ZitiEdgeRouterAdvertisedAddressVarDescription, data.Router.Edge.AdvertisedHost},
				{constants.ZitiEdgeRouterCsrCVarName, constants.ZitiEdgeRouterCsrCVarDescription, data.Router.Edge.CsrC},
				{constants.ZitiEdgeRouterCsrSTVarName, constants.ZitiEdgeRouterCsrSTVarDescription, data.Router.Edge.CsrST},
				{constants.ZitiEdgeRouterCsrLVarName, constants.ZitiEdgeRouterCsrLVarDescription, data.Router.Edge.CsrL},
				{constants.ZitiEdgeRouterCsrOVarName, constants.ZitiEdgeRouterCsrOVarDescription, data.Router.Edge.CsrO},
				{constants.ZitiEdgeRouterCsrOUVarName, constants.ZitiEdgeRouterCsrOUVarDescription, data.Router.Edge.CsrOU},
				{constants.ZitiRouterCsrSansDnsVarName, constants.ZitiRouterCsrSansDnsVarDescription, data.Router.Edge.CsrSans},
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

	var sb strings.Builder

	sb.WriteString("Creates a config file for specified component. Instead of numerous flags to be set " +
		"environment variables are used. All settings have default values but can be manually set to override " +
		"the config output.\n\nThe following environment variables can be set to override config values " +
		"(current value is displayed):\n")
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiHomeVarName, constants.ZitiHomeVarDescription, data.ZitiHome))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.PkiCtrlCertVarName, constants.PkiCtrlCertVarDescription, data.Controller.Identity.Cert))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.PkiCtrlServerCertVarName, constants.PkiCtrlServerCertVarDescription, data.Controller.Identity.ServerCert))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.PkiCtrlKeyVarName, constants.PkiCtrlKeyVarDescription, data.Controller.Identity.Key))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.PkiCtrlCAVarName, constants.PkiCtrlCAVarDescription, data.Controller.Identity.Ca))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlBindAddressVarName, constants.CtrlBindAddressVarDescription, data.Controller.Ctrl.BindAddress))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlAdvertisedAddressVarName, constants.CtrlAdvertisedAddressVarDescription, data.Controller.Ctrl.AdvertisedAddress))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlEdgeAltAdvertisedAddressVarName, constants.CtrlEdgeAltAdvertisedAddressVarDescription, data.Controller.Ctrl.AdvertisedAddress))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlAdvertisedPortVarName, constants.CtrlAdvertisedPortVarDescription, data.Controller.Ctrl.AdvertisedPort))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlEdgeBindAddressVarName, constants.CtrlEdgeBindAddressVarDescription, data.Controller.EdgeApi.Address))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlEdgeAdvertisedPortVarName, constants.CtrlEdgeAdvertisedPortVarDescription, data.Controller.EdgeApi.Port))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.PkiSignerCertVarName, constants.PkiSignerCertVarDescription, data.Controller.EdgeEnrollment.SigningCert))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.PkiSignerKeyVarName, constants.PkiSignerKeyVarDescription, data.Controller.EdgeEnrollment.SigningCertKey))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlEdgeIdentityEnrollmentDurationVarName, constants.CtrlEdgeIdentityEnrollmentDurationVarDescription, strconv.FormatInt(int64(data.Controller.EdgeEnrollment.EdgeIdentityDuration), 10)))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlEdgeRouterEnrollmentDurationVarName, constants.CtrlEdgeRouterEnrollmentDurationVarDescription, strconv.FormatInt(int64(data.Controller.EdgeEnrollment.EdgeRouterDuration), 10)))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlEdgeAdvertisedAddressVarName, constants.CtrlEdgeAdvertisedAddressVarDescription, data.Controller.Web.BindPoints.AddressAddress))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlPkiEdgeCertVarName, constants.CtrlPkiEdgeCertVarDescription, data.Controller.Web.Identity.Cert))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlPkiEdgeServerCertVarName, constants.CtrlPkiEdgeServerCertVarDescription, data.Controller.Web.Identity.ServerCert))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlPkiEdgeKeyVarName, constants.CtrlPkiEdgeKeyVarDescription, data.Controller.Web.Identity.Key))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.CtrlPkiEdgeCAVarName, constants.CtrlPkiEdgeCAVarDescription, data.Controller.Web.Identity.Ca))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.PkiAltServerCertVarName, constants.PkiAltServerCertVarDescription, data.Controller.Web.Identity.AltServerCert))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.PkiAltServerKeyVarName, constants.PkiAltServerKeyVarDescription, data.Controller.Web.Identity.AltServerKey))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterNameVarName, constants.ZitiEdgeRouterNameVarDescription, data.Router.Name))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterPortVarName, constants.ZitiEdgeRouterPortVarDescription, data.Router.Edge.Port))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterListenerBindPortVarName, constants.ZitiEdgeRouterListenerBindPortVarDescription, data.Router.Edge.ListenerBindPort))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiRouterIdentityCertVarName, constants.ZitiRouterIdentityCertVarDescription, data.Router.IdentityCert))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiRouterIdentityServerCertVarName, constants.ZitiRouterIdentityServerCertVarDescription, data.Router.IdentityServerCert))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiRouterIdentityKeyVarName, constants.ZitiRouterIdentityKeyVarDescription, data.Router.IdentityKey))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiRouterIdentityCAVarName, constants.ZitiRouterIdentityCAVarDescription, data.Router.IdentityCA))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterIPOverrideVarName, constants.ZitiEdgeRouterIPOverrideVarDescription, data.Router.Edge.IPOverride))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterAdvertisedAddressVarName, constants.ZitiEdgeRouterAdvertisedAddressVarDescription, data.Router.Edge.AdvertisedHost))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterCsrCVarName, constants.ZitiEdgeRouterCsrCVarDescription, data.Router.Edge.CsrC))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterCsrSTVarName, constants.ZitiEdgeRouterCsrSTVarDescription, data.Router.Edge.CsrST))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterCsrLVarName, constants.ZitiEdgeRouterCsrLVarDescription, data.Router.Edge.CsrL))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterCsrOVarName, constants.ZitiEdgeRouterCsrOVarDescription, data.Router.Edge.CsrO))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiEdgeRouterCsrOUVarName, constants.ZitiEdgeRouterCsrOUVarDescription, data.Router.Edge.CsrOU))
	sb.WriteString(fmt.Sprintf("%-40s %-50s %s\n", constants.ZitiRouterCsrSansDnsVarName, constants.ZitiRouterCsrSansDnsVarDescription, data.Router.Edge.CsrSans))

	cmd.Long = sb.String()

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
