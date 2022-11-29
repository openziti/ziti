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
	"github.com/openziti/edge/rest_management_api_client/certificate_authority"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/resty.v1"
	"io"
	"io/ioutil"
)

type updateCaOptions struct {
	api.Options
	verify             bool
	verifyCertPath     string
	verifyCertBytes    []byte
	nameOrId           string
	name               string
	autoCaEnrollment   bool
	ottCaEnrollment    bool
	authEnabled        bool
	identityAttributes []string
	identityNameFormat string
	tags               map[string]string

	externalIDClaim rest_model.ExternalIDClaimPatch
}

// newUpdateAuthenticatorCmd creates the 'edge controller update authenticator' command
func newUpdateCaCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := updateCaOptions{
		Options: api.Options{
			CommonOptions:      common.CommonOptions{Out: out, Err: errOut},
			OutputJSONResponse: false,
		},
		verify: false,
		externalIDClaim: rest_model.ExternalIDClaimPatch{
			Index:           I64(0),
			Location:        S(""),
			Matcher:         S(""),
			MatcherCriteria: S(""),
			Parser:          S(""),
			ParserCriteria:  S(""),
		},
	}

	cmd := &cobra.Command{
		Use:   "ca <id|name>",
		Short: "updates a ca managed by the Ziti Edge Controller",
		Long:  "updates an ca managed by the Ziti Edge Controller",
		Args: func(cmd *cobra.Command, args []string) error {
			switch {
			case len(args) == 0:
				return fmt.Errorf("ca id or name is required")
			case len(args) > 1:
				return fmt.Errorf("too many positional arguments")
			}

			options.nameOrId = args[0]

			if options.verify {
				if options.verifyCertPath == "" {
					return fmt.Errorf("--cert must be specified for --verify")
				}

				var err error
				options.verifyCertBytes, err = ioutil.ReadFile(options.verifyCertPath)

				if err != nil {
					return fmt.Errorf("could not read --cert file: %s", err)
				}
			}

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateCa(options)

			helpers.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	cmd.Flags().SetInterspersed(true)

	cmd.Flags().BoolVarP(&options.verify, "verify", "v", false, "Whether verify the CA instead of updating it, must provide --cert if used")
	cmd.Flags().StringVarP(&options.verifyCertPath, "cert", "c", "", "The certificate to verify a CA with via the --verify flag, the first certificate in the file will be used")

	cmd.Flags().StringVarP(&options.name, "name", "n", "", "The name to give the CA")

	cmd.Flags().BoolVarP(&options.authEnabled, "auth", "e", false, "Whether the CA can be used for authentication or not")
	cmd.Flags().BoolVarP(&options.ottCaEnrollment, "ottca", "o", false, "Whether the CA can be used for one-time-token CA enrollment")
	cmd.Flags().BoolVarP(&options.autoCaEnrollment, "autoca", "u", false, "Whether the CA can be used for auto CA enrollment")
	cmd.Flags().StringSliceVarP(&options.identityAttributes, "identity-attributes", "a", nil, "The roles to give to identities enrolled via the CA")
	cmd.Flags().StringVarP(&options.identityNameFormat, "identity-name-format", "f", "", "The naming format to use for identities enrolling via the CA")

	cmd.Flags().Int64VarP(options.externalIDClaim.Index, "index", "d", 0, "the index to use if multiple external ids are found, default 0")
	cmd.Flags().StringVarP(options.externalIDClaim.Location, "location", "l", "", "the location to search for external ids")
	cmd.Flags().StringVarP(options.externalIDClaim.Matcher, "matcher", "m", "", "the matcher to use at the given location")
	cmd.Flags().StringVarP(options.externalIDClaim.MatcherCriteria, "matcher-criteria", "x", "", "criteria used with the given matcher")
	cmd.Flags().StringVarP(options.externalIDClaim.Parser, "parser", "p", "", "the parser to use on found external ids")
	cmd.Flags().StringVarP(options.externalIDClaim.ParserCriteria, "parser-criteria", "z", "", "criteria used with the given parser")
	cmd.Flags().StringToStringVar(&options.tags, "tags", nil, "Custom management tags")

	options.AddCommonFlags(cmd)
	return cmd
}

func runUpdateCa(options updateCaOptions) error {
	if options.verify {
		return runVerifyCa(options)
	}

	id, err := mapCaNameToID(options.nameOrId, options.Options)

	if err != nil {
		return err
	}

	client, err := util.NewEdgeManagementClient(&options)

	if err != nil {
		return err
	}

	ca := &rest_model.CaPatch{}
	changed := false

	if options.Cmd.Flag("name").Changed {
		ca.Name = &options.name
		changed = true
	}

	if options.Cmd.Flag("auth").Changed {
		ca.IsAuthEnabled = &options.authEnabled
		changed = true
	}

	if options.Cmd.Flag("ottca").Changed {
		ca.IsOttCaEnrollmentEnabled = &options.ottCaEnrollment
		changed = true
	}

	if options.Cmd.Flag("autoca").Changed {
		ca.IsAutoCaEnrollmentEnabled = &options.autoCaEnrollment
		changed = true
	}

	if options.Cmd.Flag("identity-attributes").Changed {
		ca.IdentityRoles = options.identityAttributes
		changed = true
	}

	if options.Cmd.Flag("identity-name-format").Changed {
		ca.IdentityNameFormat = &options.identityNameFormat
		changed = true
	}

	ca.ExternalIDClaim = &rest_model.ExternalIDClaimPatch{}
	if options.Cmd.Flag("location").Changed {
		ca.ExternalIDClaim.Location = options.externalIDClaim.Location
		changed = true
	}

	if options.Cmd.Flag("index").Changed {
		ca.ExternalIDClaim.Index = options.externalIDClaim.Index
		changed = true
	}

	if options.Cmd.Flag("matcher").Changed {
		ca.ExternalIDClaim.Matcher = options.externalIDClaim.Matcher
		changed = true
	}

	if options.Cmd.Flag("matcher-criteria").Changed {
		ca.ExternalIDClaim.MatcherCriteria = options.externalIDClaim.MatcherCriteria
		changed = true
	}

	if options.Cmd.Flag("parser").Changed {
		ca.ExternalIDClaim.Parser = options.externalIDClaim.Parser
		changed = true
	}

	if options.Cmd.Flag("parser-criteria").Changed {
		ca.ExternalIDClaim.ParserCriteria = options.externalIDClaim.ParserCriteria
		changed = true
	}

	if options.Cmd.Flags().Changed("tags") {
		ca.Tags = &rest_model.Tags{
			SubTags: rest_model.SubTags{},
		}
		for k, v := range options.tags {
			ca.Tags.SubTags[k] = v
		}
		changed = true
	}

	if !changed {
		return errors.New("no values changed")
	}

	context, cancelContext := options.TimeoutContext()
	defer cancelContext()

	_, err = client.CertificateAuthority.PatchCa(&certificate_authority.PatchCaParams{
		Ca:      ca,
		ID:      id,
		Context: context,
	}, nil)

	if err != nil {
		return util.WrapIfApiError(err)
	}

	return nil
}

func runVerifyCa(options updateCaOptions) error {
	id, err := mapCaNameToID(options.nameOrId, options.Options)

	if err != nil {
		return err
	}

	_, err = doRequest("cas/"+id+"/verify", &options.Options, func(request *resty.Request, url string) (response *resty.Response, e error) {
		return request.SetHeader("Content-Type", "text/plain").
			SetBody(string(options.verifyCertBytes)).
			Post(url)
	})

	if err != nil {
		return err
	}

	return nil
}
