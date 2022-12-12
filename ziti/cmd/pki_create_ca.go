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
	"net/url"
	"strings"
)

// PKICreateCAOptions the options for the create spring command
type PKICreateCAOptions struct {
	PKICreateOptions
}

// NewCmdPKICreateCA creates a command object for the "create" command
func NewCmdPKICreateCA(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PKICreateCAOptions{
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
		Use:   "ca",
		Short: "Creates new Certificate Authority (CA) certificate",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addPKICreateCAFlags(cmd)
	return cmd
}

func (o *PKICreateCAOptions) addPKICreateCAFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.Flags.PKIRoot, "pki-root", "", "", "Directory in which PKI resides")
	cmd.Flags().StringVarP(&o.Flags.CAFile, "ca-file", "", "", "Dir/File name (within PKI_ROOT) in which to store new CA")
	cmd.Flags().StringVarP(&o.Flags.CAName, "ca-name", "", "NetFoundry Inc. Certificate Authority", "Name of CA")
	cmd.Flags().IntVarP(&o.Flags.CAExpire, "expire-limit", "", 3650, "Expiration limit in days")
	cmd.Flags().IntVarP(&o.Flags.CAMaxpath, "max-path-len", "", -1, "Intermediate maximum path length")
	cmd.Flags().IntVarP(&o.Flags.CAPrivateKeySize, "private-key-size", "", 4096, "Size of the private key")
	cmd.Flags().StringVar(&o.Flags.SpiffeID, "trust-domain", "", "An optional spiffe trust domain. spiffe:// will be automatically prefixed, if not provided")
}

// Run implements this command
func (o *PKICreateCAOptions) Run() error {
	pkiroot, err := o.ObtainPKIRoot()
	if err != nil {
		return err
	}

	o.Flags.PKI = &pki.ZitiPKI{Store: &store.Local{}}
	local := o.Flags.PKI.Store.(*store.Local)
	local.Root = pkiroot

	cafile, err := o.ObtainCAFile()
	if err != nil {
		return err
	}

	commonName := o.Flags.CAName

	filename := o.ObtainFileName(cafile, commonName)
	template := o.ObtainPKIRequestTemplate(commonName)

	template.IsCA = true

	if o.Flags.SpiffeID != "" {
		if !strings.HasPrefix(o.Flags.SpiffeID, "spiffe://") {
			o.Flags.SpiffeID = "spiffe://" + o.Flags.SpiffeID
		}
		spiffeId, err := url.Parse(o.Flags.SpiffeID)
		if err != nil {
			return errors.Wrapf(err, "unable to parse spiffe id [%v]", o.Flags.SpiffeID)
		}
		template.URIs = append(template.URIs, spiffeId)
	}

	var signer *certificate.Bundle

	req := &pki.Request{
		Name:                filename,
		Template:            template,
		IsClientCertificate: false,
		PrivateKeySize:      o.Flags.CAPrivateKeySize,
	}

	if err = o.Flags.PKI.Sign(signer, req); err != nil {
		return errors.Wrap(err, "cannot sign")
	}

	log.Infoln("Success")

	return nil

}
