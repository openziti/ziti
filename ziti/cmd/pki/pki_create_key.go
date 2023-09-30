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
	cmd2 "github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/openziti/ziti/ziti/pki/certificate"
	"github.com/openziti/ziti/ziti/pki/pki"
	"github.com/openziti/ziti/ziti/pki/store"
	"github.com/spf13/cobra"
	"io"
)

// PKICreateKeyOptions the options for the create spring command
type PKICreateKeyOptions struct {
	PKICreateOptions
}

// NewCmdPKICreateKey creates a command object for the "create" command
func NewCmdPKICreateKey(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PKICreateKeyOptions{
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
		Use:     "key",
		Short:   "Creates new private key (to be used when creating server/client certs)",
		Aliases: []string{"k"},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addPKICreateKeyFlags(cmd)
	return cmd
}

func (o *PKICreateKeyOptions) addPKICreateKeyFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.Flags.PKIRoot, "pki-root", "", "", "Directory in which PKI resides")
	cmd.Flags().StringVarP(&o.Flags.CAName, "ca-name", "", "intermediate", "Name of Intermediate CA (within PKI_ROOT) to use to sign the new Client certificate")
	cmd.Flags().StringVarP(&o.Flags.KeyFile, "key-file", "", "key", "Name of file (under chosen CA) in which to store new private key")
	cmd.Flags().IntVarP(&o.Flags.CAPrivateKeySize, "private-key-size", "", 4096, "Size of the RSA private key, ignored if -curve is set")
	cmd.Flags().StringVarP(&o.Flags.EcCurve, "curve", "", "", "If set an EC private key is generated and -private-key-size is ignored, options: P224, P256, P384, P521")
}

// Run implements this command
func (o *PKICreateKeyOptions) Run() error {

	pkiRoot, err := o.ObtainPKIRoot()
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	o.Flags.PKI = &pki.ZitiPKI{Store: &store.Local{}}
	local := o.Flags.PKI.Store.(*store.Local)
	local.Root = pkiRoot

	keyFile, err := o.ObtainKeyFile(true)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	caName, err := o.ObtainCAName(pkiRoot)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	template := o.ObtainPKIRequestTemplate("")
	var signer *certificate.Bundle

	signer, err = o.Flags.PKI.GetCA(caName)
	if err != nil {
		return fmt.Errorf("cannot locate signer: %v", err)
	}

	privateKeyOptions, err := o.ObtainPrivateKeyOptions()

	if err != nil {
		return fmt.Errorf("could not resolve private key options: %w", err)
	}

	req := &pki.Request{
		KeyName:             keyFile,
		Template:            template,
		IsClientCertificate: false,
		PrivateKeyOptions:   privateKeyOptions,
	}

	if err := o.Flags.PKI.GeneratePrivateKey(signer, req); err != nil {
		return fmt.Errorf("cannot Generate Private key: %v", err)
	}

	log.Infoln("Success")

	return nil
}
