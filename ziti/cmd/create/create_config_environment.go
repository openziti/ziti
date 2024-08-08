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
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"

	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	"github.com/openziti/ziti/ziti/constants"

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

	DisableOSVarDeclare bool
}

func (options *CreateConfigEnvironmentOptions) addFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&options.DisableOSVarDeclare, "no-shell", false, "Disable printing assignments prefixed with 'SET' (Windows) or 'export' (Unix)")
}

// NewCmdCreateConfigEnvironment creates a command object for the "environment" command
func NewCmdCreateConfigEnvironment() *cobra.Command {
	environmentOptions = &CreateConfigEnvironmentOptions{}
	controllerOptions := &CreateConfigControllerOptions{}
	
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
			SetZitiRouterIdentity(&data.Router, validateRouterName(os.Getenv(constants.ZitiEdgeRouterNameVarName)))
			// Set up other identity info
			SetControllerIdentity(&data.Controller)
			SetEdgeConfig(&data.Controller)
			SetWebConfig(&data.Controller)
			SetConsoleConfig(&data.Controller.Web.BindPoints.Console, &controllerOptions.Console)

			environmentOptions.EnvVars = []EnvVar{
				{constants.ZitiHomeVarName, constants.ZitiHomeVarDescription, data.ZitiHome},
				{constants.ZitiNetworkNameVarName, constants.ZitiNetworkNameVarDescription, data.HostnameOrNetworkName},
				{constants.PkiCtrlCertVarName, constants.PkiCtrlCertVarDescription, data.Controller.Identity.Cert},
				{constants.PkiCtrlServerCertVarName, constants.PkiCtrlServerCertVarDescription, data.Controller.Identity.ServerCert},
				{constants.PkiCtrlKeyVarName, constants.PkiCtrlKeyVarDescription, data.Controller.Identity.Key},
				{constants.PkiCtrlCAVarName, constants.PkiCtrlCAVarDescription, data.Controller.Identity.Ca},
				{constants.CtrlDatabaseFileVarName, constants.CtrlDatabaseFileVarDescription, data.Controller.Database.DatabaseFile},
				{constants.CtrlBindAddressVarName, constants.CtrlBindAddressVarDescription, data.Controller.Ctrl.BindAddress},
				{constants.CtrlAdvertisedAddressVarName, constants.CtrlAdvertisedAddressVarDescription, data.Controller.Ctrl.AdvertisedAddress},
				{constants.CtrlAdvertisedPortVarName, constants.CtrlAdvertisedPortVarDescription, data.Controller.Ctrl.AdvertisedPort},
				{constants.CtrlConsoleLocationVarName, constants.CtrlConsoleLocationVarDescription, data.Controller.Web.BindPoints.Console.Location},
				{constants.CtrlEdgeAdvertisedAddressVarName, constants.CtrlEdgeAdvertisedAddressVarDescription, data.Controller.EdgeApi.Address},
				{constants.CtrlEdgeAltAdvertisedAddressVarName, constants.CtrlEdgeAltAdvertisedAddressVarDescription, data.Controller.EdgeApi.Address},
				{constants.CtrlEdgeAdvertisedPortVarName, constants.CtrlEdgeAdvertisedPortVarDescription, data.Controller.EdgeApi.Port},
				{constants.CtrlEdgeBindAddressVarName, constants.CtrlEdgeBindAddressVarDescription, data.Controller.EdgeApi.Address},
				{constants.PkiSignerCertVarName, constants.PkiSignerCertVarDescription, data.Controller.EdgeEnrollment.SigningCert},
				{constants.PkiSignerKeyVarName, constants.PkiSignerKeyVarDescription, data.Controller.EdgeEnrollment.SigningCertKey},
				{constants.CtrlEdgeIdentityEnrollmentDurationVarName, constants.CtrlEdgeIdentityEnrollmentDurationVarDescription, strconv.FormatInt(int64(data.Controller.EdgeEnrollment.EdgeIdentityDuration.Minutes()), 10)},
				{constants.CtrlEdgeRouterEnrollmentDurationVarName, constants.CtrlEdgeRouterEnrollmentDurationVarDescription, strconv.FormatInt(int64(data.Controller.EdgeEnrollment.EdgeRouterDuration.Minutes()), 10)},
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
				{constants.ZitiEdgeRouterResolverVarName, constants.ZitiEdgeRouterResolverVarDescription, data.Router.Edge.Resolver},
				{constants.ZitiEdgeRouterDnsSvcIpRangeVarName, constants.ZitiEdgeRouterDnsSvcIpRangeVarDescription, data.Router.Edge.DnsSvcIpRange},
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
				if ! environmentOptions.DisableOSVarDeclare {
					environmentOptions.OSVarDeclare = "SET"
				}
			} else {
				environmentOptions.OSCommentPrefix = "#"
				if ! environmentOptions.DisableOSVarDeclare {
					environmentOptions.OSVarDeclare = "export"
				}
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

	sb.WriteString("Creates an env file for generating a controller or router config YAML." +
		"\nThe following can be set to override defaults:\n")
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiHomeVarName, constants.ZitiHomeVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiNetworkNameVarName, constants.ZitiNetworkNameVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.PkiCtrlCertVarName, constants.PkiCtrlCertVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.PkiCtrlServerCertVarName, constants.PkiCtrlServerCertVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.PkiCtrlKeyVarName, constants.PkiCtrlKeyVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.PkiCtrlCAVarName, constants.PkiCtrlCAVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlDatabaseFileVarName, constants.CtrlDatabaseFileVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlBindAddressVarName, constants.CtrlBindAddressVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlAdvertisedAddressVarName, constants.CtrlAdvertisedAddressVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlEdgeAltAdvertisedAddressVarName, constants.CtrlEdgeAltAdvertisedAddressVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlAdvertisedPortVarName, constants.CtrlAdvertisedPortVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlConsoleLocationVarName, constants.CtrlConsoleLocationVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlEdgeBindAddressVarName, constants.CtrlEdgeBindAddressVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlEdgeAdvertisedPortVarName, constants.CtrlEdgeAdvertisedPortVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.PkiSignerCertVarName, constants.PkiSignerCertVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.PkiSignerKeyVarName, constants.PkiSignerKeyVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlEdgeIdentityEnrollmentDurationVarName, constants.CtrlEdgeIdentityEnrollmentDurationVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlEdgeRouterEnrollmentDurationVarName, constants.CtrlEdgeRouterEnrollmentDurationVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlEdgeAdvertisedAddressVarName, constants.CtrlEdgeAdvertisedAddressVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlPkiEdgeCertVarName, constants.CtrlPkiEdgeCertVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlPkiEdgeServerCertVarName, constants.CtrlPkiEdgeServerCertVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlPkiEdgeKeyVarName, constants.CtrlPkiEdgeKeyVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.CtrlPkiEdgeCAVarName, constants.CtrlPkiEdgeCAVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.PkiAltServerCertVarName, constants.PkiAltServerCertVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.PkiAltServerKeyVarName, constants.PkiAltServerKeyVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterNameVarName, constants.ZitiEdgeRouterNameVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterPortVarName, constants.ZitiEdgeRouterPortVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterListenerBindPortVarName, constants.ZitiEdgeRouterListenerBindPortVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiRouterIdentityCertVarName, constants.ZitiRouterIdentityCertVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiRouterIdentityServerCertVarName, constants.ZitiRouterIdentityServerCertVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiRouterIdentityKeyVarName, constants.ZitiRouterIdentityKeyVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiRouterIdentityCAVarName, constants.ZitiRouterIdentityCAVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterIPOverrideVarName, constants.ZitiEdgeRouterIPOverrideVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterAdvertisedAddressVarName, constants.ZitiEdgeRouterAdvertisedAddressVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterResolverVarName, constants.ZitiEdgeRouterResolverVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterDnsSvcIpRangeVarName, constants.ZitiEdgeRouterDnsSvcIpRangeVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterCsrCVarName, constants.ZitiEdgeRouterCsrCVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterCsrSTVarName, constants.ZitiEdgeRouterCsrSTVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterCsrLVarName, constants.ZitiEdgeRouterCsrLVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterCsrOVarName, constants.ZitiEdgeRouterCsrOVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiEdgeRouterCsrOUVarName, constants.ZitiEdgeRouterCsrOUVarDescription))
	sb.WriteString(fmt.Sprintf("%-40s %-50s\n", constants.ZitiRouterCsrSansDnsVarName, constants.ZitiRouterCsrSansDnsVarDescription))

	cmd.Long = sb.String()

	environmentOptions.addFlags(cmd)
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
