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
	"github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
)

type updateAuthPolicyOptions struct {
	api.EntityOptions
	AuthPolicy rest_model.AuthPolicyPatch
	nameOrId   string
	newName    string
}

func newUpdateAuthPolicySignerCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := updateAuthPolicyOptions{
		EntityOptions: api.NewEntityOptions(out, errOut),
		AuthPolicy: rest_model.AuthPolicyPatch{
			Name: Ptr(""),
			Primary: &rest_model.AuthPolicyPrimaryPatch{
				Cert: &rest_model.AuthPolicyPrimaryCertPatch{
					AllowExpiredCerts: Ptr(false),
					Allowed:           Ptr(false),
				},
				ExtJWT: &rest_model.AuthPolicyPrimaryExtJWTPatch{
					Allowed:        Ptr(false),
					AllowedSigners: []string{},
				},
				Updb: &rest_model.AuthPolicyPrimaryUpdbPatch{
					Allowed:                Ptr(false),
					LockoutDurationMinutes: Ptr(int64(0)),
					MaxAttempts:            Ptr(int64(0)),
					MinPasswordLength:      Ptr(int64(5)),
					RequireMixedCase:       Ptr(false),
					RequireNumberChar:      Ptr(false),
					RequireSpecialChar:     Ptr(false),
				},
			},
			Secondary: &rest_model.AuthPolicySecondaryPatch{
				RequireExtJWTSigner: Ptr(""),
				RequireTotp:         Ptr(false),
			},
			Tags: &rest_model.Tags{SubTags: map[string]interface{}{}},
		},
		nameOrId: "",
		newName:  "",
	}

	cmd := &cobra.Command{
		Use:   "auth-policy <id|name>",
		Short: "updates an authentication policy managed by the Ziti Edge Controller",
		Long:  "updates an authentication policy managed by the Ziti Edge Controller",
		Args: func(cmd *cobra.Command, args []string) error {
			switch {
			case len(args) == 0:
				return fmt.Errorf("id or name is required")
			case len(args) > 1:
				return fmt.Errorf("too many positional arguments")
			}

			options.nameOrId = args[0]

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateAuthPolicySigner(options)

			helpers.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.newName, "name", "n", "", "A new name for the entity")
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

func runUpdateAuthPolicySigner(options updateAuthPolicyOptions) error {
	id, err := mapNameToID("auth-policies", options.nameOrId, options.Options)

	if err != nil {
		return err
	}

	client, err := util.NewEdgeManagementClient(&options)

	if err != nil {
		return err
	}

	changed := false

	if options.Cmd.Flag("name").Changed {
		options.AuthPolicy.Name = &options.newName
		changed = true
	} else {
		options.AuthPolicy.Name = nil
	}

	if options.Cmd.Flag("primary-cert-allowed").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.Cert.Allowed = nil
	}

	if options.Cmd.Flag("primary-cert-expired-allowed").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.Cert.AllowExpiredCerts = nil
	}

	if options.Cmd.Flag("primary-ext-jwt-allowed").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.ExtJWT.Allowed = nil
	}

	if options.Cmd.Flag("primary-ext-jwt-allowed-signers").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.ExtJWT.AllowedSigners = nil
	}

	if options.Cmd.Flag("primary-updb-allowed").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.Updb.Allowed = nil
	}

	if options.Cmd.Flag("primary-updb-req-special").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.Updb.RequireSpecialChar = nil
	}

	if options.Cmd.Flag("primary-updb-req-numbers").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.Updb.RequireNumberChar = nil
	}

	if options.Cmd.Flag("primary-updb-req-mixed-case").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.Updb.RequireMixedCase = nil
	}

	if options.Cmd.Flag("primary-updb-lockout-min").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.Updb.LockoutDurationMinutes = nil
	}

	if options.Cmd.Flag("primary-updb-max-attempts").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.Updb.MaxAttempts = nil
	}

	if options.Cmd.Flag("primary-updb-min-length").Changed {
		changed = true
	} else {
		options.AuthPolicy.Primary.Updb.MinPasswordLength = nil
	}

	if options.Cmd.Flag("secondary-req-ext-jwt-signer").Changed {
		changed = true
	} else {
		options.AuthPolicy.Secondary.RequireExtJWTSigner = nil
	}

	if options.Cmd.Flag("secondary-req-totp").Changed {
		changed = true
	} else {
		options.AuthPolicy.Secondary.RequireTotp = nil
	}

	if !changed {
		return errors.New("no values changed")
	}

	params := auth_policy.NewPatchAuthPolicyParams()
	params.AuthPolicy = &options.AuthPolicy
	params.ID = id

	_, err = client.AuthPolicy.PatchAuthPolicy(params, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	return nil
}
