/*
	Copyright NetFoundry, Inc.

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

package lets_encrypt

import (
	"fmt"
	"io"

	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

type listOptions struct {
	leOptions
	path string
}

// newListCmd creates a command object for the "controller list" command
func newListCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &listOptions{
		leOptions: leOptions{},
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Display Let's Encrypt certificates and accounts information",
		Long:  "Display Let's Encrypt certificates and accounts information",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runList(options)
			cmdhelper.CheckErr(err)
		},
	}

	return cmd
}

func runList(options *listOptions) (err error) {
	return fmt.Errorf("UNIMPLEMENTED: '%s'", "ziti pki le list")
}
