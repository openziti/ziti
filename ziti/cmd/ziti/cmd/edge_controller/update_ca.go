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
	"fmt"
	"github.com/openziti/edge/rest_management_api_client/certificate_authority"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/resty.v1"
	"io"
	"io/ioutil"
)

type updateCaOptions struct {
	edgeOptions
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
}

// newUpdateAuthenticatorCmd creates the 'edge controller update authenticator' command
func newUpdateCaCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := updateCaOptions{
		edgeOptions: edgeOptions{
			CommonOptions:      common.CommonOptions{Factory: f, Out: out, Err: errOut},
			OutputJSONResponse: false,
		},
		verify: false,
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
	cmd.Flags().StringSliceVarP(&options.identityAttributes, "identity-attributes", "i", nil, "The roles to give to identities enrolled via the CA")
	cmd.Flags().StringVarP(&options.identityNameFormat, "identity-name-format", "f", "", "The naming format to use for identities enrolling via the CA")
	options.AddCommonFlags(cmd)
	return cmd
}

func runUpdateCa(options updateCaOptions) error {
	if options.verify {
		return runVerifyCa(options)
	}

	id, err := mapCaNameToID(options.nameOrId, options.edgeOptions)

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
	id, err := mapCaNameToID(options.nameOrId, options.edgeOptions)

	if err != nil {
		return err
	}

	_, err = doRequest("cas/"+id+"/verify", &options.edgeOptions, func(request *resty.Request, url string) (response *resty.Response, e error) {
		return request.SetHeader("Content-Type", "text/plain").
			SetBody(string(options.verifyCertBytes)).
			Post(url)
	})

	if err != nil {
		return err
	}

	return nil
}
