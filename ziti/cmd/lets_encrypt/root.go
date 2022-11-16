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

package lets_encrypt

import (
	"github.com/openziti/ziti/ziti/util"
	"io"

	"github.com/spf13/cobra"
)

// NewCmdLE creates a command object for the "le" sub-command of the "pki" cmd
func NewCmdLE(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := util.NewEmptyParentCmd("le", "Interact with Let's Encrypt infra")
	populateLECommands(out, errOut, cmd)

	return cmd
}

func (options *leOptions) AddCommonFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
}

// leOptions are common options for 'pki le' commands
type leOptions struct {
	Cmd        *cobra.Command
	Args       []string
	Verbose    bool
	staging    bool
	domain     string
	acmeserver string
	email      string
	keyType    KeyTypeVar
	path       string
	accounts   bool
	names      bool
	reuseKey   bool
	port       string
	csr        string
	days       int
}

// type leFlags struct {
// }

func populateLECommands(out io.Writer, errOut io.Writer, cmd *cobra.Command) *cobra.Command {
	cmd.AddCommand(newCreateCmd(out, errOut))
	cmd.AddCommand(newRevokeCmd(out, errOut))
	cmd.AddCommand(newRenewCmd(out, errOut))
	cmd.AddCommand(newListCmd(out, errOut))
	return cmd
}
