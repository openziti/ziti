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
	"fmt"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/openziti/ziti/ziti/pki/pki"
	"github.com/openziti/ziti/ziti/pki/store"
	"github.com/spf13/cobra"
	"io"
)

// PKICreateCSROptions the options for the create spring command
type PKICreateCSROptions struct {
	PKICreateOptions
}

// NewCmdPKICreateCSR creates a command object for the "create" command
func NewCmdPKICreateCSR(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &PKICreateCSROptions{
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
		Use:   "csr",
		Short: "Creates new Certificate Signing Request (CSR)",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			cmdhelper.CheckErr(err)
		},
	}

	options.addPKICreateCSRFlags(cmd)
	return cmd
}

func (o *PKICreateCSROptions) addPKICreateCSRFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&o.Flags.CSRFile, "csr-file", "", "csr", "File in which to store new CSR")
	cmd.Flags().StringVarP(&o.Flags.CSRName, "csr-name", "", "NetFoundry Inc. CSR", "Name of CSR")
	cmd.Flags().StringVarP(&o.Flags.KeyName, "key-name", "", "", "Name of file that contains private key for CSR")
	cmd.Flags().IntVarP(&o.Flags.CAExpire, "expire-limit", "", 365, "Expiration limit in days")
	cmd.Flags().IntVarP(&o.Flags.CAMaxpath, "max-path-len", "", -1, "Intermediate maximum path length")
	cmd.Flags().IntVarP(&o.Flags.CAPrivateKeySize, "private-key-size", "", 4096, "Size of the private key")
}

// Run implements this command
func (o *PKICreateCSROptions) Run() error {

	pkiroot, err := o.ObtainPKIRoot()
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	o.Flags.PKI = &pki.ZitiPKI{Store: &store.Local{}}
	local := o.Flags.PKI.Store.(*store.Local)
	local.Root = pkiroot

	csrfile, err := o.ObtainCSRFile()
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	keyname, err := o.ObtainKeyName(pkiroot)
	if err != nil {
		return fmt.Errorf("%s", err)
	}

	commonName := o.Flags.CSRName
	template := o.ObtainPKICSRRequestTemplate(commonName)

	key, err := o.Flags.PKI.GetPrivateKey(keyname, keyname)
	if err != nil {
		return fmt.Errorf("Cannot locate private key: %v", err)
	}

	if err := o.Flags.PKI.CSR(keyname, csrfile, *template, key); err != nil {
		return fmt.Errorf("Cannot create CSR: %v", err)
	}

	log.Infoln("Success")

	return nil
}
