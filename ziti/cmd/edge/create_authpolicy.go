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
	"github.com/openziti/edge-api/rest_management_api_client/auth_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/ziti/cmd/api"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"io"
)

type createAuthPolicyOptions struct {
	api.EntityOptions
	AuthPolicy rest_model.AuthPolicyCreate
}

func newCreateAuthPolicyCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createAuthPolicyOptions{
		EntityOptions: api.NewEntityOptions(out, errOut),
		AuthPolicy: rest_model.AuthPolicyCreate{
			Name: Ptr(""),
			Primary: &rest_model.AuthPolicyPrimary{
				Cert: &rest_model.AuthPolicyPrimaryCert{
					AllowExpiredCerts: Ptr(false),
					Allowed:           Ptr(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWT{
					Allowed:        Ptr(false),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdb{
					Allowed:                Ptr(false),
					LockoutDurationMinutes: Ptr(int64(0)),
					MaxAttempts:            Ptr(int64(0)),
					MinPasswordLength:      Ptr(int64(5)),
					RequireMixedCase:       Ptr(false),
					RequireNumberChar:      Ptr(false),
					RequireSpecialChar:     Ptr(false),
				},
			},
			Secondary: &rest_model.AuthPolicySecondary{
				RequireExtJWTSigner: Ptr(""),
				RequireTotp:         Ptr(false),
			},
			Tags: &rest_model.Tags{SubTags: map[string]interface{}{}},
		},
	}

	cmd := &cobra.Command{
		Use:   "auth-policy <name>",
		Short: "creates an authentication policy managed by the Ziti Edge Controller",
		Long:  "creates an authentication policy managed by the Ziti Edge Controller",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("requires 1 arg, received %d", len(args))
			}

			options.AuthPolicy.Name = &args[0]

			return nil

		},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateAuthPolicy(options)

			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVar(options.AuthPolicy.Primary.Cert.Allowed, "primary-cert-allowed", false, "Enable certificate authentication")
	cmd.Flags().BoolVar(options.AuthPolicy.Primary.Cert.AllowExpiredCerts, "primary-cert-expired-allowed", false, "Allow expired certificates")
	cmd.Flags().BoolVar(options.AuthPolicy.Primary.ExtJWT.Allowed, "primary-ext-jwt-allowed", false, "Allow external JWT authentication")
	cmd.Flags().StringArrayVar(&options.AuthPolicy.Primary.ExtJWT.AllowedSigners, "primary-ext-jwt-allowed-signers", []string{}, "Allow specific JWT signers")
	cmd.Flags().BoolVar(options.AuthPolicy.Primary.Updb.Allowed, "primary-updb-allowed", false, "Allow username/password db authentication")
	cmd.Flags().BoolVar(options.AuthPolicy.Primary.Updb.RequireSpecialChar, "primary-updb-req-special", false, "Require special characters in passwords")
	cmd.Flags().BoolVar(options.AuthPolicy.Primary.Updb.RequireNumberChar, "primary-updb-req-numbers", false, "Require numbers in passwords")
	cmd.Flags().BoolVar(options.AuthPolicy.Primary.Updb.RequireMixedCase, "primary-updb-req-mixed-case", false, "Require mixed case in passwords")
	cmd.Flags().Int64Var(options.AuthPolicy.Primary.Updb.LockoutDurationMinutes, "primary-updb-lockout-min", 0, "Lockout duration minutes after max attempts, 0=forever")
	cmd.Flags().Int64Var(options.AuthPolicy.Primary.Updb.MaxAttempts, "primary-updb-max-attempts", 0, "Number of invalid authentication attempts, 0=unlimited")
	cmd.Flags().Int64Var(options.AuthPolicy.Primary.Updb.MinPasswordLength, "primary-updb-min-length", 5, "Minimum password length")

	cmd.Flags().StringVar(options.AuthPolicy.Secondary.RequireExtJWTSigner, "secondary-req-ext-jwt-signer", "", "JWT required on every request")
	cmd.Flags().BoolVar(options.AuthPolicy.Secondary.RequireTotp, "secondary-req-totp", false, "MFA TOTP enrollment required")
	options.AddCommonFlags(cmd)

	return cmd
}

func runCreateAuthPolicy(options *createAuthPolicyOptions) (err error) {
	managementClient, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return err
	}

	for k, v := range options.GetTags() {
		options.AuthPolicy.Tags.SubTags[k] = v
	}

	if options.AuthPolicy.Secondary.RequireExtJWTSigner != nil && *options.AuthPolicy.Secondary.RequireExtJWTSigner == "" {
		options.AuthPolicy.Secondary.RequireExtJWTSigner = nil
	}

	params := auth_policy.NewCreateAuthPolicyParams()
	params.AuthPolicy = &options.AuthPolicy

	resp, err := managementClient.AuthPolicy.CreateAuthPolicy(params, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	checkId := resp.GetPayload().Data.ID

	if _, err = fmt.Fprintf(options.Out, "%v\n", checkId); err != nil {
		panic(err)
	}

	return err
}
