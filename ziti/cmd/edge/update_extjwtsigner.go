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
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
)

type updateExtJwtOptions struct {
	api.EntityOptions
	ExtJwtSigner rest_model.ExternalJWTSignerPatch
	nameOrId     string
	newName      string
	JwksEndpoint string
}

// newUpdateExtJwtSignerCmd creates the 'edge controller update authenticator' command
func newUpdateExtJwtSignerCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	jwksEndpoint := strfmt.URI("")

	options := updateExtJwtOptions{
		EntityOptions: api.NewEntityOptions(out, errOut),
		ExtJwtSigner: rest_model.ExternalJWTSignerPatch{
			Audience:        Ptr(""),
			CertPem:         Ptr(""),
			ClaimsProperty:  Ptr(""),
			Enabled:         Ptr(true),
			ExternalAuthURL: Ptr(""),
			Issuer:          Ptr(""),
			JwksEndpoint:    &jwksEndpoint,
			Kid:             Ptr(""),
			Name:            Ptr(""),
			Tags:            &rest_model.Tags{SubTags: map[string]interface{}{}},
			UseExternalID:   Ptr(false),
			Scopes:          []string{},
			ClientID:        Ptr(""),
		},
		nameOrId:     "",
		newName:      "",
		JwksEndpoint: "",
	}

	cmd := &cobra.Command{
		Use:   "ext-jwt-signer <id|name> [-u <jwksEndpoint>|-p <cert pem>|-f <cert file>] [-n <nameName> -a <audience> -c <claimProperty> --client-id <clientId> --scope <scope1> --scope <scopeN> -xe]",
		Short: "updates an external jwt signer managed by the Ziti Edge Controller",
		Long:  "updates an external jwt signer managed by the Ziti Edge Controller",
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
			err := runUpdateExtJwtSigner(options)

			helpers.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	cmd.Flags().SetInterspersed(true)

	cmd.Flags().StringVarP(&options.newName, "name", "n", "", "The name to give the CA")

	options.AddCommonFlags(cmd)
	cmd.Flags().BoolVarP(options.ExtJwtSigner.UseExternalID, "use-external-id", "x", true, "Matches identity external ids rather than internal ids")
	cmd.Flags().BoolVarP(options.ExtJwtSigner.Enabled, "enabled", "e", true, "Enable this entity")
	cmd.Flags().StringVarP(&options.JwksEndpoint, "jwks-endpoint", "u", "", "A valid URI for a target JWKS endpoint, not usable with -p or -f")
	cmd.Flags().StringVarP(options.ExtJwtSigner.ClaimsProperty, "claims-property", "c", "sub", "The JWT property matched to identity internal/external ids")
	cmd.Flags().StringVarP(options.ExtJwtSigner.Audience, "audience", "a", "", "The expected audience of the incoming JWTs")
	cmd.Flags().StringVarP(options.ExtJwtSigner.CertPem, "cert-pem", "p", "", "A public certificate PEM for the signer, not usable with -u, -f")
	cmd.Flags().StringVarP(options.ExtJwtSigner.CertPem, "cert-file", "f", "", "A public certificate PEM file, not usable with -u, -p")
	cmd.Flags().StringVarP(options.ExtJwtSigner.ExternalAuthURL, "external-auth-url", "y", "", "The URL that users are directed to obtain a JWT")
	cmd.Flags().StringVarP(options.ExtJwtSigner.Kid, "kid", "k", "", "The KID for the signer, required if using -p or -f")
	cmd.Flags().StringVarP(options.ExtJwtSigner.Issuer, "issuer", "l", "", "The issuer for the signer")
	cmd.Flags().StringVarP(options.ExtJwtSigner.ClientID, "client-id", "", "", "The client id for OIDC that should be used")
	cmd.Flags().StringSliceVarP(&options.ExtJwtSigner.Scopes, "scopes", "", nil, "The scopes for OIDC that should be used")
	return cmd
}

func runUpdateExtJwtSigner(options updateExtJwtOptions) error {
	id, err := mapNameToID("external-jwt-signers", options.nameOrId, options.Options)

	if err != nil {
		return err
	}

	client, err := util.NewEdgeManagementClient(&options)

	if err != nil {
		return err
	}

	changed := false

	if options.Cmd.Flag("name").Changed {
		options.ExtJwtSigner.Name = &options.newName
		changed = true
	} else {
		options.ExtJwtSigner.Name = nil
	}

	if options.Cmd.Flag("issuer").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.Issuer = nil
	}

	if options.Cmd.Flag("use-external-id").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.UseExternalID = nil
	}

	if options.Cmd.Flag("enabled").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.Enabled = nil
	}

	if options.Cmd.Flag("jwks-endpoint").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.JwksEndpoint = nil
	}

	if options.Cmd.Flag("claims-property").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.ClaimsProperty = nil
	}

	if options.Cmd.Flag("audience").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.Audience = nil
	}

	if options.Cmd.Flag("cert-pem").Changed || options.Cmd.Flag("cert-file").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.CertPem = nil
	}

	if options.Cmd.Flag("external-auth-url").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.ExternalAuthURL = nil
	}

	if options.Cmd.Flag("kid").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.Kid = nil
	}

	if options.Cmd.Flag("scopes").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.Scopes = nil
	}

	if options.Cmd.Flag("client-id").Changed {
		changed = true
	} else {
		options.ExtJwtSigner.ClientID = nil
	}

	if options.TagsProvided() {
		options.ExtJwtSigner.Tags = &rest_model.Tags{
			SubTags: rest_model.SubTags{},
		}
		for k, v := range options.GetTags() {
			options.ExtJwtSigner.Tags.SubTags[k] = v
		}
		changed = true
	}

	if !changed {
		return errors.New("no values changed")
	}

	params := external_jwt_signer.NewPatchExternalJWTSignerParams()
	params.ExternalJWTSigner = &options.ExtJwtSigner
	params.ID = id

	_, err = client.ExternalJWTSigner.PatchExternalJWTSigner(params, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	return nil
}
