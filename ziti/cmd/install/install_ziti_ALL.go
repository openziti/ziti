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
	"io"

	"github.com/blang/semver"
	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/spf13/cobra"
)

var (
	installZitiALLLong = templates.LongDesc(`
		Installs all Ziti apps that have not been installed already
`)

	installZitiALLExample = templates.Examples(`
		# Install all the Ziti apps 
		ziti install all
	`)
)

// InstallZitiALLOptions the options for the upgrade ziti-channel command
type InstallZitiALLOptions struct {
	InstallOptions

	Version string
}

// NewCmdInstallZitiALL defines the command
func NewCmdInstallZitiALL(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallZitiALLOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "all",
		Short:   "Installs all the Ziti apps that have not been installed already",
		Aliases: []string{"*"},
		Long:    installZitiALLLong,
		Example: installZitiALLExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&options.Version, "version", "v", "", "The specific version to install")
	options.AddCommonFlags(cmd)
	return cmd
}

func (o *InstallZitiALLOptions) install(zitiApp string) error {

	newVersion, err := o.getLatestZitiAppVersion(version.GetBranch(), zitiApp)
	if err != nil {
		return err
	}

	if o.Version != "" {
		newVersion, err = semver.Make(o.Version)
	}

	log.Infoln("Attempting to install '" + zitiApp + "'  version: " + newVersion.String())

	return o.installZitiApp(version.GetBranch(), zitiApp, false, newVersion.String())
}

// Run implements the command
func (o *InstallZitiALLOptions) Run() error {
	err := o.install(c.ZITI_CONTROLLER)
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}

	err = o.install(c.ZITI_PROX_C)
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}

	err = o.install(c.ZITI_ROUTER)
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}

	err = o.install(c.ZITI_TUNNEL)
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}

	err = o.install(c.ZITI_EDGE_TUNNEL)
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}

	return nil
}
