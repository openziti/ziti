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

package ext_jwt_signer

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/openziti/ziti/ziti/cmd/ops/verify/ext-jwt-signer/oidc"
)

func NewVerifyExtJwtSignerCmd(out io.Writer, errOut io.Writer, initialContext context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ext-jwt-signer",
		Short: "test if an external JWT signer is correctly configured",
		Long:  "tests and verifies an external JWT signer is configured correctly",
	}

	oidcCmd := oidc.NewOidcVerificationCmd(out, errOut, initialContext)
	cmd.AddCommand(oidcCmd)

	return cmd
}
