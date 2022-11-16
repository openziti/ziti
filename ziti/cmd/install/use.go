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
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/openziti/ziti/ziti/util"
	"io"

	"github.com/blang/semver"
	"github.com/openziti/ziti/common/version"
	"github.com/spf13/cobra"
)

// UseOptions are the flags for delete commands
type UseOptions struct {
	InstallOptions

	Version string
	Branch  string
}

// NewCmdUse creates the command
func NewCmdUse(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &UseOptions{
		InstallOptions: InstallOptions{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:   "use [branch]",
		Short: "switch between branches of Ziti",
		Long: `
'ziti use' fetches the list of currently available branch-build names from artifactory,
presents them in a chooser-list, and once one is selected, will switch the current 'ziti' binary for the 
chosen one. This is useful for swapping between different feature branches, or back and forth from a release
build to a feature-branch build.
`,
		Aliases: []string{},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
		Hidden:     true,
	}

	cmd.Flags().StringVarP(&options.Version, "version", "v", "", "The specific version to use")
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
	cmd.Flags().StringVarP(&options.Branch, "branch", "b", "", "Name of branch to switch to")
	cmd.Flags().BoolVarP(&options.Staging, "staging", "", false, "Install/Upgrade components from the ziti-staging repo")

	return cmd
}

func (o *UseOptions) install(branch string, zitiApp string) error {
	newVersion, err := o.getLatestZitiAppVersionForBranch(branch, zitiApp)
	if err != nil {
		log.Infoln("Attempt to fetch latest version of '" + zitiApp + "' for branch '" + branch + "' failed: " + err.Error())

		// Special-case branch fallback (to master) when dealing with ziti-prox-c
		if zitiApp == c.ZITI_PROX_C || zitiApp == c.ZITI_EDGE_TUNNEL {
			branch = "master"
			newVersion, err = o.getLatestZitiAppVersionForBranch(branch, zitiApp)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if o.Version != "" {
		newVersion, err = semver.Make(o.Version)
	}

	log.Infoln("Attempting to install '" + zitiApp + "'  version: " + newVersion.String() + " from branch '" + branch + "'")

	return o.installZitiApp(branch, zitiApp, true, newVersion.String())
}

// Run implements the command
func (o *UseOptions) Run() error {

	fmt.Println("Current source branch is: ", version.GetBranch())

	branch := o.Branch
	if branch == "" {
		list, err := o.getCurrentZitiSnapshotList()
		branch, err = util.PickName(list, "Which Branch would you like to switch to: ")
		if err != nil {
			return err
		}
	}

	if o.Staging {
		if o.Branch != "main" {
			log.Errorf("Error: --staging can only be used with --branch of 'main'. You specified '%s'", branch)
			return nil
		}
	}

	err := o.install(branch, c.ZITI)
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}
	err = o.install(branch, c.ZITI_CONTROLLER)
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}
	err = o.install(branch, c.ZITI_ROUTER)
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}
	err = o.install(branch, c.ZITI_TUNNEL)
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}

	err = o.installZitiProxC("")
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}

	err = o.installZitiEdgeTunnel("")
	if err != nil {
		log.Errorf("Error: install failed  %s \n", err.Error())
	}

	return nil
}
