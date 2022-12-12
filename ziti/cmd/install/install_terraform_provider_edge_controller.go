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

package install

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/internal/log"
	"io"

	"github.com/blang/semver"
	"github.com/spf13/cobra"
)

var (
	installTerraformProviderEdgeControllerLong = templates.LongDesc(`
		Installs the Terraform Provider for the Ziti Edge Controller
`)

	installTerraformProviderEdgeControllerExample = templates.Examples(`
		# Install the Terraform Provider for the Ziti Edge Controller 
		ziti install terraform-provider-edgecontroller
	`)
)

// InstallTerraformProviderEdgeControllerOptions the options for the upgrade ziti-tunnel command
type InstallTerraformProviderEdgeControllerOptions struct {
	InstallOptions

	Version string
	Branch  string
}

// NewCmdInstallTerraformProviderEdgeController defines the command
func NewCmdInstallTerraformProviderEdgeController(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallTerraformProviderEdgeControllerOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "terraform-provider-edgecontroller",
		Short:   "Installs the Terraform Provider for the Ziti Edge Controller",
		Aliases: []string{"tpec"},
		Long:    installTerraformProviderEdgeControllerLong,
		Example: installTerraformProviderEdgeControllerExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&options.Version, "version", "v", "", "The specific version to install")
	cmd.Flags().StringVarP(&options.Branch, "branch", "b", "master", "The specific version to install")
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
	return cmd
}

// Run implements the command
func (o *InstallTerraformProviderEdgeControllerOptions) Run() error {
	newVersion, err := o.getLatestTerraformProviderVersion(o.Branch, c.TERRAFORM_PROVIDER_EDGE_CONTROLLER)
	if err != nil {
		return err
	}

	if o.Version != "" {
		newVersion, err = semver.Make(o.Version)
	}

	log.Infoln("Attempting to install Terraform Provider '" + c.TERRAFORM_PROVIDER_EDGE_CONTROLLER + "' version: " + newVersion.String())

	return o.installTerraformProvider(o.Branch, c.TERRAFORM_PROVIDER_EDGE_CONTROLLER, false, newVersion.String())
}
