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

package edge

import (
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"io"
)

// logoutOptions are the flags for logout commands
type logoutOptions struct {
	api.Options
}

// newLogoutCmd creates the command
func newLogoutCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &logoutOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "logs out of a Ziti Edge Controller instance",
		Long:  `logout removes stored credentials for a given identity`,
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
	options.AddCommonFlags(cmd)

	return cmd
}

// Run implements this command
func (o *logoutOptions) Run() error {
	config, configFile, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	id := config.GetIdentity()
	o.Printf("Removing identity '%v' from %v\n", id, configFile)
	delete(config.EdgeIdentities, id)
	return util.PersistRestClientConfig(config)
}
