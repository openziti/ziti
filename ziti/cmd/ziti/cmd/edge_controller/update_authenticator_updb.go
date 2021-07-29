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
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
)
import cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"

type updateUpdbOptions struct {
	edgeOptions
	identity         string
	newPassword      string
	currentPassword  string
	identityPassword string
	self             bool
}

func newUpdateAuthenticatorUpdb(idType string, options edgeOptions) *cobra.Command {
	updbOptions := updateUpdbOptions{
		edgeOptions: options,
	}
	cmd := &cobra.Command{
		Use:   idType + " (-i <identityIdOrName> -p <newPassword>) | (-c <currentPassword> -n <newPassword>)",
		Short: "allows an admin to set the updb authenticator of an identity or an authenticated session to update its authenticator ",
		Long:  "The -i and -p flags are used in conjunction to set the password of an already existing updb authenticator. The -o and -n flags are used to update the current authenticated sessions updb authenticator",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateUpdbPassword(idType, &updbOptions)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().StringVarP(&updbOptions.identity, "identity", "i", "", "The id or name of the identity to update a password for, may be used with --password (requires admin)")
	cmd.Flags().StringVarP(&updbOptions.identityPassword, "password", "p", "", "The password to set for an identity, may be used with --identity (requires admin), if not supplied a valued will be prompted for")

	cmd.Flags().BoolVarP(&updbOptions.self, "self", "s", false, "Specify updating a password for the currently active identity can use --old and --new to supply passwords")
	cmd.Flags().StringVarP(&updbOptions.currentPassword, "current", "c", "", "The current password of the identity logged in, may be used with --self, if not supplied a valued will be prompted for")
	cmd.Flags().StringVarP(&updbOptions.newPassword, "new", "n", "", "The new password to use for the current identity logged in, may be used with --self, if not supplied a valued will be prompted for")
	return cmd
}

func runUpdateUpdbPassword(idType string, options *updateUpdbOptions) error {

	if options.identity != "" && options.self {
		return errors.New("--self and --identity cannot be mixed")
	}

	if options.identity != "" {
		return setIdentityPassword(options.identity, options.identityPassword, options.edgeOptions)
	}

	if options.self {
		return updateSelfPassword(options.currentPassword, options.newPassword, options.edgeOptions)
	}

	return errors.New("invalid arguments, requires --self or --identity, see help for details")
}

func updateSelfPassword(current string, new string, options edgeOptions) error {
	var err error
	if current == "" {
		if current, err = term.PromptPassword("Enter your current password: ", false); err != nil {
			return err
		}
	}

	if new == "" {
		if new, err = term.PromptPassword("Enter your new password: ", false); err != nil {
			return err
		}
	}

	passwordData := gabs.New()
	setJSONValue(passwordData, current, "currentPassword")
	setJSONValue(passwordData, new, "password")

	respEnvelope, err := util.EdgeControllerList("current-identity/authenticators", map[string][]string{"filter": {`method="updb"`}}, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)

	if err != nil {
		return err
	}

	authenticators, err := respEnvelope.S("data").Children()

	if err != nil {
		return err
	}

	if len(authenticators) == 0 {
		return errors.New("no updb authenticator found for the current identity")
	} else if len(authenticators) > 1 {
		return errors.New("too many updb authenticator found for the current identity")
	}

	_, err = patchEntityOfType("current-identity/authenticators/"+authenticators[0].Path("id").Data().(string), passwordData.String(), &options)

	if err != nil {
		return err
	}

	return nil
}

func setIdentityPassword(identity, password string, options edgeOptions) error {
	id, err := mapIdentityNameToID(identity, options)

	if err != nil {
		return err
	}

	client, err := util.NewEdgeManagementClient(&options)

	if err != nil {
		return err
	}
	filter := fmt.Sprintf(`identity="%s" and method="updb"`, id)

	result, err := client.Authenticator.ListAuthenticators(&authenticator.ListAuthenticatorsParams{
		Filter: &filter,
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

	authenticatorId := result.Payload.Data[0].ID

	if password == "" {
		if password, err = term.PromptPassword("Enter the identity's new password : ", false); err != nil {
			return err
		}

		verifyPassword := ""
		if verifyPassword, err = term.PromptPassword("Enter password again: ", false); err != nil {
			return err
		}

		if verifyPassword != password {
			return errors.New("passwords did not match")
		}
	}

	passwordData := gabs.New()
	setJSONValue(passwordData, password, "password")

	_, err = patchEntityOfType(fmt.Sprintf("authenticators/%s", *authenticatorId), passwordData.String(), &options)

	if err != nil {
		return err
	}

	return nil
}
