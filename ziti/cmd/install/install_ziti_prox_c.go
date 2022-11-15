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
	installZitiProxCLong = templates.LongDesc(`
		Installs the Ziti ProxC app if it has not been installed already
`)

	installZitiProxCExample = templates.Examples(`
		# Install the Ziti ProxC app 
		ziti install ziti-prox-c
	`)
)

// InstallZitiProxCOptions the options for the upgrade ziti-prox-c command
type InstallZitiProxCOptions struct {
	InstallOptions

	Version string
}

// NewCmdInstallZitiProxC defines the command
func NewCmdInstallZitiProxC(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &InstallZitiProxCOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "ziti-prox-c",
		Short:   "Installs the Ziti ProxC app - if it has not been installed already",
		Aliases: []string{"proxc"},
		Long:    installZitiProxCLong,
		Example: installZitiProxCExample,
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

func (o *InstallOptions) installZitiProxC(targetVersion string) error {
	if targetVersion != "" {
		version, err := semver.Make(targetVersion)
		if err != nil {
			return err
		}
		return o.findVersionAndInstallGitHubRelease(c.ZITI_PROX_C, c.ZITI_SDK_C_GITHUB, false, version.String())
	}

	release, err := o.getHighestVersionGitHubReleaseInfo(c.ZITI_SDK_C_GITHUB)
	if err != nil {
		return err
	}
	log.Infoln("Attempting to install '" + c.ZITI_PROX_C + "' version: " + release.SemVer.String())
	return o.installGitHubRelease(c.ZITI_PROX_C, false, release)
}

// Run implements the command
func (o *InstallZitiProxCOptions) Run() error {
	return o.installZitiProxC(o.Version)
}
