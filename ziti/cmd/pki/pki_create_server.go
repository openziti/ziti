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
	"net/url"
)

// PKICreateServerOptions the options for the create spring command
type PKICreateServerOptions struct {
	PKICreateOptions
}

// NewCmdPKICreateServer creates a command object for the "create" command
func NewCmdPKICreateServer(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PKICreateServerOptions{
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
		Use:     "server",
		Short:   "Creates new Server certificate (signed by previously created Intermediate-chain)",
		Aliases: []string{"s"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addPKICreateServerFlags(cmd)
	return cmd
}

const FlagCaServerName = "server-name"

func (o *PKICreateServerOptions) addPKICreateServerFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.Flags.PKIRoot, "pki-root", "", "", "Directory in which PKI resides")
	cmd.Flags().StringVarP(&o.Flags.CAName, "ca-name", "", "intermediate", "Name of Intermediate CA (within PKI_ROOT) to use to sign the new Server certificate")
	cmd.Flags().StringVarP(&o.Flags.ServerFile, "server-file", "", "server", "Name of file (under chosen CA) in which to store new Server certificate and private key")
	cmd.Flags().StringVarP(&o.Flags.KeyFile, "key-file", "", "", "Name of file (under chosen CA) containing private key to use when generating Server certificate")
	cmd.Flags().StringVarP(&o.Flags.ServerName, FlagCaServerName, "", "NetFoundry Inc. Server", "Common Name (CN) to use for new Server certificate")
	cmd.Flags().StringSliceVar(&o.Flags.DNSName, "dns", []string{}, "DNS name(s) to add to Subject Alternate Name (SAN) for new Server certificate")
	cmd.Flags().StringSliceVar(&o.Flags.IP, "ip", []string{}, "IP addr(s) to add to Subject Alternate Name (SAN) for new Server certificate")
	cmd.Flags().IntVarP(&o.Flags.CAExpire, "expire-limit", "", 365, "Expiration limit in days")
	cmd.Flags().IntVarP(&o.Flags.CAMaxPath, "max-path-len", "", -1, "Intermediate maximum path length")
	cmd.Flags().IntVarP(&o.Flags.CAPrivateKeySize, "private-key-size", "", 4096, "Size of the RSA private key, ignored if -curve is set")
	cmd.Flags().StringVarP(&o.Flags.EcCurve, "curve", "", "", "If set an EC private key is generated and -private-key-size is ignored, options: P224, P256, P384, P521")
	cmd.Flags().StringVar(&o.Flags.SpiffeID, "spiffe-id", "", "Optionally provide the path portion of a SPIFFE id. The trust domain will be taken from the signing certificate.")
	cmd.Flags().BoolVar(&o.Flags.AllowOverwrite, "allow-overwrite", false, "Allow overwrite existing certs")
}

// Run implements this command
func (o *PKICreateServerOptions) Run() error {
	IPs, DNSNames, err := o.ObtainIPsAndDNSNames()
	if err != nil {
		return err
	}

	pkiroot, err := o.ObtainPKIRoot()
	if err != nil {
		return err
	}

	o.Flags.PKI = &pki.ZitiPKI{Store: &store.Local{}}
	local := o.Flags.PKI.Store.(*store.Local)
	local.Root = pkiroot

	if !o.Cmd.Flags().Changed(FlagCaServerName) {
		o.Flags.ServerName = o.Flags.ServerName + " " + idgen.New()
	}

	commonName := o.Flags.ServerName

	serverCertFile, err := o.ObtainServerCertFile()
	if err != nil {
		return err
	}

	filename := o.ObtainFileName(serverCertFile, commonName)
	template := o.ObtainPKIRequestTemplate(commonName)

	template.IsCA = false
	template.IPAddresses = IPs
	template.DNSNames = DNSNames

	var signer *certificate.Bundle

	caname, err := o.ObtainCAName(pkiroot)
	if err != nil {
		return err
	}

	keyFile, err := o.ObtainKeyFile(false)
	if err != nil {
		return err
	}

	signer, err = o.Flags.PKI.GetCA(caname)
	if err != nil {
		return errors.Wrap(err, "cannot locate signer")
	}

	if o.Flags.SpiffeID != "" {
		var trustDomain *url.URL
		for _, uri := range signer.Cert.URIs {
			if uri.Scheme == "spiffe" {
				if trustDomain != nil {
					return errors.New("signing cert contained multiple spiffe ids, which is not allowed")
				}
				trustDomain = uri
			}
		}

		if trustDomain == nil {
			return errors.New("signing cert doesn't have a spiffe id. unknown trust domain")
		}

		spiffId := *trustDomain
		spiffId.Path = o.Flags.SpiffeID
		template.URIs = append(template.URIs, &spiffId)
	}

	privateKeyOptions, err := o.ObtainPrivateKeyOptions()

	if err != nil {
		return fmt.Errorf("could not resolve private key options: %w", err)
	}

	req := &pki.Request{
		Name:                filename,
		KeyName:             keyFile,
		Template:            template,
		IsClientCertificate: false,
		PrivateKeyOptions:   privateKeyOptions,
		AllowOverwrite:      o.Flags.AllowOverwrite,
	}

	if err := o.Flags.PKI.Sign(signer, req); err != nil {
		return errors.Wrap(err, "cannot sign")
	}

	// Concat the newly-created server cert with the intermediate cert to create a server.chain.pem file
	if err := o.Flags.PKI.Chain(signer, req); err != nil {
		return errors.Wrap(err, "unable to generate cert chain")
	}

	log.Infoln("Success")

	return nil
}
