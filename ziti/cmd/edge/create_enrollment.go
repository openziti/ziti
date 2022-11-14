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
	"context"
	"fmt"
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge/rest_management_api_client/enrollment"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"io"
	"time"
)

func newCreateEnrollmentCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use: "enrollment",
	}

	cmd.AddCommand(newCreateEnrollmentOtt(out, errOut))
	cmd.AddCommand(newCreateEnrollmentOttCa(out, errOut))
	cmd.AddCommand(newCreateEnrollmentUpdb(out, errOut))

	return cmd
}

type createEnrollmentOptions struct {
	api.Options
	name     string
	duration int64
}

func newCreateEnrollmentOtt(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createEnrollmentOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:   "ott <identityIdOrName> [-duration <minutes>]",
		Short: "creates a one-time-token (ott) enrollment for the given identity",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args

			err := runCreateEnrollmentOtt(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)
	cmd.Flags().Int64VarP(&options.duration, "duration", "d", 30, "the duration of time the enrollment should valid for")

	return cmd
}

func newCreateEnrollmentOttCa(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createEnrollmentOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:   "ottca <identityIdOrName> <caIdOrName> [-duration <minutes>]",
		Short: "creates a one-time-token ca (ottca) enrollment for the given identity and ca",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args

			err := runCreateEnrollmentOttCa(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)
	cmd.Flags().Int64VarP(&options.duration, "duration", "d", 30, "the duration of time the enrollment should valid for")

	return cmd
}

func runCreateEnrollmentOtt(options *createEnrollmentOptions) error {
	managementClient, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return err
	}

	identityId, err := mapNameToID("identities", options.Args[0], options.Options)

	if err != nil {
		return err
	}

	method := rest_model.EnrollmentCreateMethodOtt
	expiresAt := strfmt.DateTime(time.Now().Add(time.Duration(options.duration) * time.Minute))

	params := &enrollment.CreateEnrollmentParams{
		Enrollment: &rest_model.EnrollmentCreate{
			ExpiresAt:  &expiresAt,
			IdentityID: &identityId,
			Method:     &method,
		},
		Context: context.Background(),
	}

	resp, err := managementClient.Enrollment.CreateEnrollment(params, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	enrollmentID := resp.GetPayload().Data.ID

	if _, err = fmt.Fprintf(options.Out, "%v\n", enrollmentID); err != nil {
		panic(err)
	}

	return err
}

func runCreateEnrollmentOttCa(options *createEnrollmentOptions) error {
	managementClient, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return err
	}

	identityId, err := mapNameToID("identities", options.Args[0], options.Options)

	if err != nil {
		return err
	}

	caId, err := mapNameToID("cas", options.Args[1], options.Options)

	if err != nil {
		return err
	}

	method := rest_model.EnrollmentCreateMethodOttca
	expiresAt := strfmt.DateTime(time.Now().Add(time.Duration(options.duration) * time.Minute))

	params := &enrollment.CreateEnrollmentParams{
		Enrollment: &rest_model.EnrollmentCreate{
			ExpiresAt:  &expiresAt,
			IdentityID: &identityId,
			Method:     &method,
			CaID:       &caId,
		},
		Context: context.Background(),
	}

	resp, err := managementClient.Enrollment.CreateEnrollment(params, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	enrollmentID := resp.GetPayload().Data.ID

	if _, err = fmt.Fprintf(options.Out, "%v\n", enrollmentID); err != nil {
		panic(err)
	}

	return err
}

func newCreateEnrollmentUpdb(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createEnrollmentOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:   "updb <identityIdOrName> <username> [-duration <minutes>]",
		Short: "creates a username password (updb) enrollment for the given identity",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args

			err := runCreateEnrollmentUpdb(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)
	cmd.Flags().Int64VarP(&options.duration, "duration", "d", 30, "the duration of time the enrollment should valid for")

	return cmd
}

func runCreateEnrollmentUpdb(options *createEnrollmentOptions) error {
	managementClient, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return err
	}

	identityId, err := mapNameToID("identities", options.Args[0], options.Options)

	if err != nil {
		return err
	}

	username := options.Args[1]

	method := rest_model.EnrollmentCreateMethodUpdb
	expiresAt := strfmt.DateTime(time.Now().Add(time.Duration(options.duration) * time.Minute))

	params := &enrollment.CreateEnrollmentParams{
		Enrollment: &rest_model.EnrollmentCreate{
			ExpiresAt:  &expiresAt,
			IdentityID: &identityId,
			Method:     &method,
			Username:   &username,
		},
		Context: context.Background(),
	}

	resp, err := managementClient.Enrollment.CreateEnrollment(params, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	enrollmentID := resp.GetPayload().Data.ID

	if _, err = fmt.Fprintf(options.Out, "%v\n", enrollmentID); err != nil {
		panic(err)
	}

	return err
}
