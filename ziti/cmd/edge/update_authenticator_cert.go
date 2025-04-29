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
	"errors"
	"fmt"
	"github.com/openziti/edge-api/rest_management_api_client/authenticator"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/ziti/cmd/api"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"strings"
)

type updateCertOptions struct {
	*api.Options
	authenticatorId string
	requestExtend   bool
	requestKeyRoll  bool
}

func newUpdateAuthenticatorCert(idType string, options api.Options) *cobra.Command {
	certOptions := updateCertOptions{
		Options: &options,
	}
	cmd := &cobra.Command{
		Use:   idType + " <authenticatorId> [--requestExtend] [--requestKeyRoll]",
		Short: "allows an admin to set request a cert authenticator be extended and optionally key rolled",
		Long:  "Request a specific certificate authenticator to --requestExtend or --requestKeyRoll, --requestKeyRoll implies --requestExtend",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateCert(&certOptions)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVarP(&certOptions.requestExtend, "requestExtend", "e", false, "Specify the certificate authenticator should be flagged for extension")
	cmd.Flags().BoolVarP(&certOptions.requestKeyRoll, "requestKeyRoll", "r", false, "Specify the certificate authenticator should be flagged for key rolling, implies --requestExtend")
	return cmd
}

func runUpdateCert(options *updateCertOptions) error {
	id := strings.TrimSpace(options.Args[0])

	if id == "" {
		return errors.New("no authenticator id specified or was blank")
	}
	if !options.requestKeyRoll && !options.requestExtend {
		return errors.New("--requestExtend and --requestKeyRoll are both false, no work")
	}

	managementClient, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	params := &authenticator.RequestExtendAuthenticatorParams{
		ID: id,
		RequestExtendAuthenticator: &rest_model.RequestExtendAuthenticator{
			RollKeys: options.requestKeyRoll,
		},
	}

	_, err = managementClient.Authenticator.RequestExtendAuthenticator(params, nil)

	if err != nil {
		return fmt.Errorf("authentication request extend failed: %w", util.WrapIfApiError(err))
	}

	return nil
}
