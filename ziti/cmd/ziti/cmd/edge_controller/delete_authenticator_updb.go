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

package edge_controller

import (
	"context"
	"fmt"
	"github.com/openziti/edge/rest_management_api_client/authenticator"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
)
import cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"

type deleteUpdbOptions struct {
	edgeOptions
}

func newDeleteAuthenticatorUpdb(idType string, options *edgeOptions) *cobra.Command {
	updbOptions := deleteUpdbOptions{
		edgeOptions: *options,
	}

	cmd := &cobra.Command{
		Use:   idType + " <identityNameOrId>",
		Short: "deletes an identity's " + idType + " authenticator managed by the Ziti Edge Controller",
		Long:  "deletes a identity's " + idType + " authenticator managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runDeleteUpdb(args[0], &updbOptions)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	return cmd
}

func runDeleteUpdb(idOrName string, options *deleteUpdbOptions) error {
	identityId, err := mapIdentityNameToID(idOrName, options.edgeOptions)

	if err != nil {
		return err
	}

	client, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return err
	}
	filter := fmt.Sprintf(`identity="%s" and method="updb"`, identityId)

	result, err := client.Authenticator.ListAuthenticators(&authenticator.ListAuthenticatorsParams{
		Filter:  &filter,
		Context: context.Background(),
	}, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	if len(result.Payload.Data) != 1 {
		return fmt.Errorf("incorrect number of results, expected 1, got %d", len(result.Payload.Data))
	}

	if result.Payload.Data[0].ID == nil {
		return fmt.Errorf("authenticator contained a nil id")
	}

	if *result.Payload.Data[0].ID == "" {
		return fmt.Errorf("authenticator contained an empty id")
	}

	authenticatorId := *result.Payload.Data[0].ID

	_, err = deleteEntityOfType(fmt.Sprintf("authenticators/%s", authenticatorId), "", &options.edgeOptions)

	if err != nil {
		return err
	}

	return nil
}
