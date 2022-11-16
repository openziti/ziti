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

package cmd

import (
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
				CommonOptions: CommonOptions{
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

func (o *PKICreateIntermediateOptions) addPKICreateIntermediateFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.Flags.PKIRoot, "pki-root", "", "", "Directory in which PKI resides")
	cmd.Flags().StringVarP(&o.Flags.CAName, "ca-name", "", "ca", "Name of CA (within PKI_ROOT) to use to sign the new Intermediate CA")
	cmd.Flags().StringVarP(&o.Flags.IntermediateFile, "intermediate-file", "", "intermediate", "Dir/File name (within PKI_ROOT) in which to store new Intermediate CA")
	cmd.Flags().StringVarP(&o.Flags.IntermediateName, "intermediate-name", "", "NetFoundry Inc. Intermediate CA", "Common Name (CN) to use for new Intermediate CA")
	cmd.Flags().IntVarP(&o.Flags.CAExpire, "expire-limit", "", 3650, "Expiration limit in days")
	cmd.Flags().IntVarP(&o.Flags.CAMaxpath, "max-path-len", "", 0, "Intermediate maximum path length")
	cmd.Flags().IntVarP(&o.Flags.CAPrivateKeySize, "private-key-size", "", 4096, "Size of the private key")
}

// Run implements this command
func (o *PKICreateIntermediateOptions) Run() error {
	pkiroot, err := o.ObtainPKIRoot()
	if err != nil {
		return err
	}

	o.Flags.PKI = &pki.ZitiPKI{Store: &store.Local{}}
	local := o.Flags.PKI.Store.(*store.Local)
	local.Root = pkiroot

	intermediatefile, err := o.ObtainIntermediateCAFile()
	if err != nil {
		return err
	}

	commonName := o.Flags.IntermediateName

	filename := o.ObtainFileName(intermediatefile, commonName)
	template := o.ObtainPKIRequestTemplate(commonName)

	template.IsCA = true

	var signer *certificate.Bundle

	caname, err := o.ObtainCAName(pkiroot)
	if err != nil {
		return err
	}

	signer, err = o.Flags.PKI.GetCA(caname)
	if err != nil {
		return errors.Wrap(err, "cannot locate signer")
	}

	for _, uri := range signer.Cert.URIs {
		if uri.Scheme == "spiffe" {
			template.URIs = append(template.URIs, uri)
		}
	}

	req := &pki.Request{
		Name:                filename,
		Template:            template,
		IsClientCertificate: false,
		PrivateKeySize:      o.Flags.CAPrivateKeySize,
	}

	if err := o.Flags.PKI.Sign(signer, req); err != nil {
		return errors.Wrap(err, "cannot sign")
	}

	log.Infoln("Success")

	return nil
}
