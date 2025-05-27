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
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge-api/rest_management_api_client/authenticator"
	"github.com/openziti/edge-api/rest_management_api_client/enrollment"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/ziti/cmd/api"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"strings"
	"time"
)

type updateCertOptions struct {
	*api.Options
	requestExtend  bool
	requestKeyRoll bool
	reEnroll       bool
	duration       string
}

func newUpdateAuthenticatorCert(idType string, options api.Options) *cobra.Command {
	certOptions := updateCertOptions{
		Options: &options,
	}
	cmd := &cobra.Command{
		Use:   idType + " <authenticatorId> [--request-extend] [--request-key-roll] [--re-enroll]",
		Short: "allows an admin to set request a cert authenticator be extended and optionally key rolled or re-enrolled",
		Long:  "Request a specific certificate authenticator to --request-extend or --request-key-roll, --request-key-roll implies --request-extend which are both mutually exclusive with --re-enroll.",
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
	cmd.Flags().BoolVarP(&certOptions.requestExtend, "request-extend", "e", false, "Specify the certificate authenticator should be flagged for extension")
	cmd.Flags().BoolVarP(&certOptions.requestKeyRoll, "request-key-roll", "r", false, "Specify the certificate authenticator should be flagged for key rolling, implies --request-extend")
	cmd.Flags().BoolVarP(&certOptions.reEnroll, "re-enroll", "x", false, "Removes this authenticator and replaces it with an enrollment token of the same type that will be valid for 24h or the value set via --duration")
	cmd.Flags().StringVarP(&certOptions.duration, "duration", "d", "24h", "The duration of time a new enrollment from --re-enroll should be valid for, only used if --re-enroll is set (e.g. 60s, 5m, 24h)")
	return cmd
}

func runUpdateCert(options *updateCertOptions) error {
	id := strings.TrimSpace(options.Args[0])

	if id == "" {
		return errors.New("no authenticator id specified or was blank")
	}
	if !options.requestKeyRoll && !options.requestExtend && !options.reEnroll {
		return errors.New("--request-extend, --request-key-roll, and --re-enroll are all false, no work to do")
	}

	if (options.requestKeyRoll || options.requestExtend) && options.reEnroll {
		return errors.New("--request-extend and --request-key-roll are mutually exclusive with --re-enroll")
	}

	var duration time.Duration = 0

	if options.duration != "" {
		var err error
		duration, err = time.ParseDuration(options.duration)

		if err != nil {
			return fmt.Errorf("could not parse duration '%s': %w", options.duration, err)
		}
	}

	if options.reEnroll {
		return reEnrollAuthenticator(id, duration, options.Options)
	}

	return requestExtendAuthenticator(id, options)
}

func requestExtendAuthenticator(id string, options *updateCertOptions) error {
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

func reEnrollAuthenticator(id string, duration time.Duration, options *api.Options) error {
	managementClient, err := util.NewEdgeManagementClient(options)

	if duration < time.Minute*1 {
		return fmt.Errorf("duration must be at least 1 minute (1m or 60s), got %s", duration.String())
	}

	if err != nil {
		return util.WrapIfApiError(err)
	}

	expiresAt := strfmt.DateTime(time.Now().Add(duration))

	reEnrollParams := &authenticator.ReEnrollAuthenticatorParams{
		ID: id,
		ReEnroll: &rest_model.ReEnroll{
			ExpiresAt: &expiresAt,
		},
	}

	resp, err := managementClient.Authenticator.ReEnrollAuthenticator(reEnrollParams, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	detailEnrollmentParams := &enrollment.DetailEnrollmentParams{
		ID: resp.GetPayload().Data.ID,
	}

	detailResp, err := managementClient.Enrollment.DetailEnrollment(detailEnrollmentParams, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	output := ""
	if options.OutputJSONResponse {
		jsonOut, _ := detailResp.GetPayload().Data.MarshalBinary()
		output = string(jsonOut)
	} else {
		token := stringz.OrEmpty(detailResp.GetPayload().Data.Token)
		output = fmt.Sprintf("created enrollment id: %s\n\ntoken: \n%s\n", resp.GetPayload().Data.ID, token)
	}

	_, _ = options.Out.Write([]byte(output))

	return nil
}
