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
	"github.com/spf13/cobra"
	"io"
)

// newUpdateAuthenticatorCmd creates the 'edge controller update authenticator' command
func newUpdateAuthenticatorCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := api.Options{
		CommonOptions:      common.CommonOptions{Out: out, Err: errOut},
		OutputJSONResponse: false,
	}

	cmd := &cobra.Command{
		Use:   "authenticator",
		Short: "updates an authenticator for an identity managed by the Ziti Edge Controller",
		Long:  "updates an authenticator for an identity managed by the Ziti Edge Controller",
	}

	cmd.AddCommand(newUpdateAuthenticatorUpdb("updb", options))

	return cmd
}
