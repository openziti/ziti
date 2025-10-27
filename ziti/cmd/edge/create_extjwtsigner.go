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
	"os"
	"strings"

	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge-api/rest_management_api_client/external_jwt_signer"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/ziti/cmd/api"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type createExtJwtSignerOptions struct {
	api.EntityOptions
	ExtJwtSigner rest_model.ExternalJWTSignerCreate
	CertFilePath string
	JwksEndpoint string
	TargetToken  string
}

// newCreateExtJwtSignerCmd creates the 'edge controller create ca local' command for the given entity type
func newCreateExtJwtSignerCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	jwksEndpoint := strfmt.URI("")
	options := &createExtJwtSignerOptions{
		EntityOptions: api.NewEntityOptions(out, errOut),
		ExtJwtSigner: rest_model.ExternalJWTSignerCreate{
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
	}

	cmd := &cobra.Command{
		Use:     "ext-jwt-signer <name> <issuer> (-u <jwksEndpoint>|-p <cert pem>|-f <cert file>) [-a <audience> -c <claimProperty> --client-id <clientId> --scope <scope1> --scope <scopeN> -xe --target-token=ACCESS|ID]",
		Short:   "creates an external JWT signer managed by the Ziti Edge Controller",
		Long:    "creates an external JWT signer managed by the Ziti Edge Controller",
		Aliases: []string{"external-jwt-signer"},
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return fmt.Errorf("requires 2 arg, received %d", len(args))
			}

			options.ExtJwtSigner.Name = &args[0]
			options.ExtJwtSigner.Issuer = &args[1]

			return nil

		},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateExtJwtSigner(options)

			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVarP(options.ExtJwtSigner.UseExternalID, "external-id", "x", true, "Matches identity external ids rather than internal ids")
	cmd.Flags().BoolVarP(options.ExtJwtSigner.Enabled, "enabled", "e", true, "Enable this entity")
	cmd.Flags().StringVarP(&options.JwksEndpoint, "jwks-endpoint", "u", "", "A valid URI for a target JWKS endpoint, not usable with -p or -f")
	cmd.Flags().StringVarP(options.ExtJwtSigner.ClaimsProperty, "claims-property", "c", "sub", "The JWT property matched to identity internal/external ids")
	cmd.Flags().StringVarP(options.ExtJwtSigner.Audience, "audience", "a", "", "The expected audience of the incoming JWTs")
	cmd.Flags().StringVarP(options.ExtJwtSigner.CertPem, "cert-pem", "p", "", "A public certificate PEM for the signer, not usable with -u, -f")
	cmd.Flags().StringVarP(&options.CertFilePath, "cert-file", "f", "", "A public certificate PEM file, not usable with -u, -p")
	cmd.Flags().StringVarP(options.ExtJwtSigner.ExternalAuthURL, "external-auth-url", "y", "", "The URL that users are directed to obtain a JWT")
	cmd.Flags().StringVarP(options.ExtJwtSigner.Kid, "kid", "k", "", "The KID for the signer, required if using -p or -f")
	cmd.Flags().StringVarP(options.ExtJwtSigner.ClientID, "client-id", "", "", "The client id for OIDC that should be used")
	cmd.Flags().StringSliceVarP(&options.ExtJwtSigner.Scopes, "scopes", "", nil, "The scopes for OIDC that should be used")
	cmd.Flags().StringVarP(&options.TargetToken, "target-token", "", "ACCESS", "The target token SDKs should use, defaults to ACCESS")
	cmd.Flags().BoolVarP(&options.ExtJwtSigner.EnrollToCertEnabled, "enroll-to-cert", "", false, "Enable enrollment to certificate authentication, defaults to false")
	cmd.Flags().BoolVarP(&options.ExtJwtSigner.EnrollToTokenEnabled, "enroll-to-token", "", false, "Enable enrollment to a token authentication, defaults to false")
	cmd.Flags().StringVarP(&options.ExtJwtSigner.EnrollNameClaimsSelector, "enroll-name-claims-selector", "", "", "The claims JSON pointer selector or top level property to use for the name of enrolling identities, defaults to /sub")
	cmd.Flags().StringVarP(&options.ExtJwtSigner.EnrollAttributeClaimsSelector, "enroll-attr-claims-selector", "", "", "The claims JSON pointer selector or top level property to use for the attributes of enrolling identities")
	cmd.Flags().StringVarP(&options.ExtJwtSigner.EnrollAuthPolicyID, "enroll-auth-policy", "", "", "The name or ID of the authentication policy to use for enrolling identities, defaults to `default`")

	options.AddCommonFlags(cmd)

	return cmd
}

func runCreateExtJwtSigner(options *createExtJwtSignerOptions) (err error) {
	managementClient, err := util.NewEdgeManagementClient(options)

	if err != nil {
		return err
	}

	hasJwks := options.JwksEndpoint != ""
	hasCertPem := options.ExtJwtSigner.CertPem != nil && *options.ExtJwtSigner.CertPem != ""
	hasCertPath := options.CertFilePath != ""

	if (hasJwks && !(!hasCertPem && !hasCertPath)) || (hasCertPem && !(!hasJwks && !hasCertPath)) || (hasCertPath && !(!hasJwks && !hasCertPem)) {
		return errors.New("must specify only one certificate source (JWKS, cert file, or inline cert PEM")
	}

	if options.ExtJwtSigner.Audience == nil || *options.ExtJwtSigner.Audience == "" {
		return errors.New("audience must be specified")
	}

	if options.ExtJwtSigner.ClaimsProperty != nil && *options.ExtJwtSigner.ClaimsProperty == "" {
		return errors.New("claims property must not be an empty string")
	}

	if options.TargetToken == "" {
		options.ExtJwtSigner.TargetToken = Ptr(rest_model.TargetTokenACCESS)
	} else if options.TargetToken != "" && (options.TargetToken != string(rest_model.TargetTokenACCESS) && options.TargetToken != string(rest_model.TargetTokenID)) {
		return fmt.Errorf("target-token must be %s or %s", string(rest_model.TargetTokenACCESS), string(rest_model.TargetTokenID))
	} else {
		options.ExtJwtSigner.TargetToken = Ptr(rest_model.TargetToken(options.TargetToken))
	}

	if options.JwksEndpoint != "" {
		jwks := strfmt.URI(options.JwksEndpoint)
		options.ExtJwtSigner.JwksEndpoint = &jwks
		options.ExtJwtSigner.CertPem = nil
		options.ExtJwtSigner.Kid = nil
	} else {
		options.ExtJwtSigner.JwksEndpoint = nil
	}

	if hasCertPath {
		pem, err := os.ReadFile(options.CertFilePath)
		if err != nil {
			return fmt.Errorf("could not read certificate file path %s: %w", options.CertFilePath, err)
		}
		pemStr := string(pem)
		options.ExtJwtSigner.CertPem = &pemStr
	}

	for k, v := range options.GetTags() {
		options.ExtJwtSigner.Tags.SubTags[k] = v
	}

	if options.ExtJwtSigner.ClientID != nil && *options.ExtJwtSigner.ClientID == "" {
		options.ExtJwtSigner.ClientID = nil
	}

	if options.ExtJwtSigner.Scopes != nil && len(options.ExtJwtSigner.Scopes) == 0 {
		options.ExtJwtSigner.Scopes = nil
	}

	var cleanedScopes []string

	for _, curScope := range options.ExtJwtSigner.Scopes {
		if strings.TrimSpace(curScope) != "" {
			cleanedScopes = append(cleanedScopes, curScope)
		}
	}
	options.ExtJwtSigner.Scopes = cleanedScopes

	if options.ExtJwtSigner.EnrollAuthPolicyID != "" {
		idOrName := options.ExtJwtSigner.EnrollAuthPolicyID

		resolvedId, err := api.MapNameToID(util.EdgeAPI, "auth-policies", &options.EntityOptions.Options, idOrName)
		if err != nil {
			return err
		}

		options.ExtJwtSigner.EnrollAuthPolicyID = resolvedId
	}

	params := external_jwt_signer.NewCreateExternalJWTSignerParams()
	params.ExternalJWTSigner = &options.ExtJwtSigner

	resp, err := managementClient.ExternalJWTSigner.CreateExternalJWTSigner(params, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	checkId := resp.GetPayload().Data.ID

	if _, err = fmt.Fprintf(options.Out, "%v\n", checkId); err != nil {
		panic(err)
	}

	return err
}
