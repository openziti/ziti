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

type renewOptions struct {
	leOptions
	path string
}

// newListCmd creates a command object for the "controller list" command
func newRenewCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &renewOptions{
		leOptions: leOptions{},
	}

	cmd := &cobra.Command{
		Use:   "renew",
		Short: "Renew a Let's Encrypt certificate",
		Long:  "Renew a Let's Encrypt certificate",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runRenew(options)
			cmdhelper.CheckErr(err)
		},
	}

	return cmd
}

func runRenew(options *renewOptions) (err error) {
	return fmt.Errorf("UNIMPLEMENTED: '%s'", "ziti pki le renew")
}
