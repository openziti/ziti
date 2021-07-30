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
	"errors"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/edge/rest_management_api_client/authenticator"
	"github.com/openziti/foundation/util/term"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
)

type createAuthenticatorUpdb struct {
	edgeOptions
	idOrName string
	password string
	username string
}

func newCreateAuthenticatorUpdb(idType string, options edgeOptions) *cobra.Command {
	updbOptions := &createAuthenticatorUpdb{edgeOptions: options}

	cmd := &cobra.Command{
		Use:     idType + " <identityIdOrName> <username> [<password>]",
		Short:   "creates an identity's " + idType + " authenticator.",
		Long:    "Creates a updb authenticator for an identity which will allow the identity to authenticate with a username/password combination. If <password> is omitted it will be prompted for.",
		Example: "ziti edge controller create authenticator updb \"David Bright\" \"dbright\" \"@$yh3Hh3h4\"",
		Args: func(cmd *cobra.Command, args []string) error {
			minArgs := 2
			maxArgs := 3
			if len(args) < minArgs {
				return fmt.Errorf("requires at least %d arg(s), only received %d", minArgs, len(args))
			}

			if len(args) > maxArgs {
				return fmt.Errorf("requires at most %d arg(s), received %d", maxArgs, len(args))
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			updbOptions.idOrName = args[0]
			updbOptions.username = args[1]

			if len(args) > 2 {
				updbOptions.password = args[2]
			}

			err := runCreateIdentityPassword(idType, updbOptions)
			helpers.CheckErr(err)
		},
		SuggestFor: []string{},
	}
	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	return cmd
}

func runCreateIdentityPassword(idType string, options *createAuthenticatorUpdb) error {
	if options.idOrName == "" {
		return errors.New("an identity must be specified")
	}

	identityId, err := mapIdentityNameToID(options.idOrName, options.edgeOptions)

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

	if len(result.Payload.Data) != 0 {
		return fmt.Errorf("updb authentictor already exists, update or remove it instead")
	}

	if options.password == "" {
		if options.password, err = term.PromptPassword("Enter password: ", false); err != nil {
			return err
		}
		verifyPassword := ""
		if verifyPassword, err = term.PromptPassword("Enter password again: ", false); err != nil {
			return err
		}

		if verifyPassword != options.password {
			return errors.New("passwords did not match")
		}
	}

	authenticatorData := gabs.New()
	setJSONValue(authenticatorData, identityId, "identityId")
	setJSONValue(authenticatorData, options.password, "password")
	setJSONValue(authenticatorData, options.username, "username")
	setJSONValue(authenticatorData, "updb", "method")

	if _, err = createEntityOfType("authenticators", authenticatorData.String(), &options.edgeOptions); err != nil {
		return err
	}

	return nil
}
