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

// useOptions are the flags for use commands
type useOptions struct {
	api.Options
}

// newUseCmd creates the command
func newUseCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &useOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "use <identity>",
		Short: "changes which saved login to use with a Ziti Edge Controller instance",
		Args:  cobra.RangeArgs(0, 1),
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
func (o *useOptions) Run() error {
	config, configFile, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	// If no identities specified, list known identities
	if len(o.Args) == 0 {
		id := config.GetIdentity()
		for k, v := range config.EdgeIdentities {
			o.Printf("id: %12v | current: %5v | read-only: %5v | urL: %v\n", k, k == id, v.ReadOnly, v.Url)
		}
		return nil
	}

	id := o.Args[0]
	config.Default = id
	o.Printf("Setting identity '%v' as default in %v\n", id, configFile)
	return util.PersistRestClientConfig(config)
}
