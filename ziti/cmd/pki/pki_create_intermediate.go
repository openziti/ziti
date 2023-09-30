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

package pki

import (
	"fmt"
	"github.com/openziti/ziti/controller/idgen"
	cmd2 "github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/openziti/ziti/ziti/pki/certificate"
	"github.com/openziti/ziti/ziti/pki/pki"
	"github.com/openziti/ziti/ziti/pki/store"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
)

// PKICreateIntermediateOptions the options for the create spring command
type PKICreateIntermediateOptions struct {
	PKICreateOptions
}

// NewCmdPKICreateIntermediate creates a command object for the "create" command
func NewCmdPKICreateIntermediate(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PKICreateIntermediateOptions{
		PKICreateOptions: PKICreateOptions{
			PKIOptions: PKIOptions{
				CommonOptions: cmd2.CommonOptions{
					Out: out,
					Err: errOut,
				},
			},
		},
	}

	cmd := &cobra.Command{
		Use:     "intermediate",
		Short:   "Creates new Intermediate-chain certificate (signed by previously created CA)",
		Aliases: []string{"i"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addPKICreateIntermediateFlags(cmd)
	return cmd
}

const FlagCaIntermediateName = "intermediate-name"

func (o *PKICreateIntermediateOptions) addPKICreateIntermediateFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.Flags.PKIRoot, "pki-root", "", "", "Directory in which PKI resides")
	cmd.Flags().StringVarP(&o.Flags.CAName, "ca-name", "", "ca", "Name of CA (within PKI_ROOT) to use to sign the new Intermediate CA")
	cmd.Flags().StringVarP(&o.Flags.IntermediateFile, "intermediate-file", "", "intermediate", "Dir/File name (within PKI_ROOT) in which to store new Intermediate CA")
	cmd.Flags().StringVarP(&o.Flags.IntermediateName, FlagCaIntermediateName, "", "NetFoundry Inc. Intermediate CA", "Common Name (CN) to use for new Intermediate CA")
	cmd.Flags().IntVarP(&o.Flags.CAExpire, "expire-limit", "", 3650, "Expiration limit in days")
	cmd.Flags().IntVarP(&o.Flags.CAMaxPath, "max-path-len", "", 0, "Intermediate maximum path length")
	cmd.Flags().IntVarP(&o.Flags.CAPrivateKeySize, "private-key-size", "", 4096, "Size of the RSA private key, ignored if -curve is set")
	cmd.Flags().StringVarP(&o.Flags.EcCurve, "curve", "", "", "If set an EC private key is generated and -private-key-size is ignored, options: P224, P256, P384, P521")
}

// Run implements this command
func (o *PKICreateIntermediateOptions) Run() error {
	pkiRoot, err := o.ObtainPKIRoot()
	if err != nil {
		return err
	}

	o.Flags.PKI = &pki.ZitiPKI{Store: &store.Local{}}
	local := o.Flags.PKI.Store.(*store.Local)
	local.Root = pkiRoot

	intermediateFile, err := o.ObtainIntermediateCAFile()
	if err != nil {
		return err
	}

	if !o.Cmd.Flags().Changed(FlagCaIntermediateName) {
		o.Flags.IntermediateName = o.Flags.IntermediateName + " " + idgen.New()
	}

	commonName := o.Flags.IntermediateName

	filename := o.ObtainFileName(intermediateFile, commonName)
	template := o.ObtainPKIRequestTemplate(commonName)

	template.IsCA = true

	var signer *certificate.Bundle

	caName, err := o.ObtainCAName(pkiRoot)
	if err != nil {
		return err
	}

	signer, err = o.Flags.PKI.GetCA(caName)
	if err != nil {
		return errors.Wrap(err, "cannot locate signer")
	}

	for _, uri := range signer.Cert.URIs {
		if uri.Scheme == "spiffe" {
			template.URIs = append(template.URIs, uri)
		}
	}

	privateKeyOptions, err := o.ObtainPrivateKeyOptions()

	if err != nil {
		return fmt.Errorf("could not resolve private key options: %w", err)
	}

	req := &pki.Request{
		Name:                filename,
		Template:            template,
		IsClientCertificate: false,
		PrivateKeyOptions:   privateKeyOptions,
	}

	if err := o.Flags.PKI.Sign(signer, req); err != nil {
		return errors.Wrap(err, "cannot sign")
	}

	// Concat the newly-created intermediate cert with the signing cert to create an intermediate.chain.pem file
	if err := o.Flags.PKI.Chain(signer, req); err != nil {
		return errors.Wrap(err, "unable to generate cert chain")
	}

	log.Infoln("Success")

	return nil
}
