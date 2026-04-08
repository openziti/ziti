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
	"fmt"
	"io"

	"github.com/openziti/edge-api/rest_management_api_client/revocation"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/spf13/cobra"
)

// newCreateRevocationCmd returns the parent "create revocation" command with
// identity, api-session, and jti sub-commands.
func newCreateRevocationCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revocation",
		Short: "creates a revocation entry managed by the Ziti Edge Controller",
		Long:  "Creates a revocation entry managed by the Ziti Edge Controller",
	}

	cmd.AddCommand(newCreateRevocationIdentityCmd(out, errOut))
	cmd.AddCommand(newCreateRevocationApiSessionCmd(out, errOut))
	cmd.AddCommand(newCreateRevocationJtiCmd(out, errOut))

	return cmd
}

func newCreateRevocationIdentityCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := api.NewEntityOptions(out, errOut)

	cmd := &cobra.Command{
		Use:   "identity <identityId>",
		Short: "revokes all tokens for an identity",
		Long:  "Creates a revocation entry for an identity ID, invalidating all tokens issued to that identity before this point.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return runCreateRevocation(&options, args[0], rest_model.RevocationTypeEnumIDENTITY)
		},
	}

	options.AddCommonFlags(cmd)
	return cmd
}

func newCreateRevocationApiSessionCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := api.NewEntityOptions(out, errOut)

	cmd := &cobra.Command{
		Use:   "api-session <apiSessionId>",
		Short: "revokes all tokens for an API session",
		Long:  "Creates a revocation entry for an API session ID (the z_asid claim), invalidating all tokens issued to that session before this point.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return runCreateRevocation(&options, args[0], rest_model.RevocationTypeEnumAPISESSION)
		},
	}

	options.AddCommonFlags(cmd)
	return cmd
}

func newCreateRevocationJtiCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := api.NewEntityOptions(out, errOut)

	cmd := &cobra.Command{
		Use:   "jti <jti>",
		Short: "revokes a specific token by its JTI",
		Long:  "Creates a revocation entry for a specific JWT token ID (the jti claim).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return runCreateRevocation(&options, args[0], rest_model.RevocationTypeEnumJTI)
		},
	}

	options.AddCommonFlags(cmd)
	return cmd
}

func runCreateRevocation(options *api.EntityOptions, id string, revocationType rest_model.RevocationTypeEnum) error {
	managementClient, err := util.NewEdgeManagementClient(options)
	if err != nil {
		return err
	}

	params := revocation.NewCreateRevocationParams()
	params.Revocation = &rest_model.RevocationCreate{
		ID:   &id,
		Type: &revocationType,
	}

	resp, err := managementClient.Revocation.CreateRevocation(params, nil)
	if err != nil {
		return util.WrapIfApiError(err)
	}

	if _, err = fmt.Fprintf(options.Out, "%v\n", resp.GetPayload().Data.ID); err != nil {
		panic(err)
	}

	return nil
}
