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

package fabric

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
)

// removeIdentityOptions are the flags for removeIdentity commands
type removeIdentityOptions struct {
	common.CommonOptions
}

// newRemoveIdentityCmd creates the command
func newRemoveIdentityCmd(p common.OptionsProvider) *cobra.Command {
	options := &removeIdentityOptions{
		CommonOptions: p(),
	}

	cmd := &cobra.Command{
		Use:   "remove-identity",
		Short: "remove an identity for a Ziti Controller instance",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&common.CliIdentity, "cli-identity", "i", "", "Specify the saved identity you want the CLI to use when connect to the controller with")

	return cmd
}

// Run implements this command
func (o *removeIdentityOptions) Run() error {
	config, configFile, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	id := config.GetIdentity()
	o.Printf("Removing fabric identity '%v' from %v\n", id, configFile)
	delete(config.FabricIdentities, id)
	return util.PersistRestClientConfig(config)
}
