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

package verify

import (
	"context"
	"io"

	"github.com/openziti/ziti/ziti/cmd/ops/verify/ext-jwt-signer"
	"github.com/spf13/cobra"
)

func NewVerifyCommand(out io.Writer, errOut io.Writer, initialContext context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "a group of commands used to verify an overlay is setup properly",
		Long:  "a group of commands used to verify an overlay is setup properly",
	}

	cmd.AddCommand(NewVerifyNetwork(out, errOut))
	cmd.AddCommand(NewVerifyTraffic(out, errOut))
	cmd.AddCommand(ext_jwt_signer.NewVerifyExtJwtSignerCmd(out, errOut, initialContext))

	return cmd
}
