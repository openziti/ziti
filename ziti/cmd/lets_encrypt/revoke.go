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

package lets_encrypt

import (
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/internal/log"
	"io"

	"github.com/spf13/cobra"
)

// newListCmd creates a command object for the "controller list" command
func newRevokeCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &leOptions{}

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke a Let's Encrypt certificate",
		Long:  "Revoke a Let's Encrypt certificate",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runRevoke(options)
			cmdhelper.CheckErr(err)
		},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().StringVarP(&options.domain, "domain", "d", "", "Domain for which Cert is being generated (e.g. me.example.com)")
	cmd.MarkFlagRequired("domain")
	cmd.Flags().StringVarP(&options.path, "path", "p", "", "Directory where data is stored")
	cmd.MarkFlagRequired("path")
	cmd.Flags().StringVarP(&options.email, "email", "e", "openziti@netfoundry.io", "Email used for registration and recovery contact")
	cmd.Flags().StringVarP(&options.acmeserver, "acmeserver", "a", acmeProd, "ACME CA hostname")
	cmd.Flags().BoolVarP(&options.staging, "staging", "s", false, "Enable creation of 'staging' Certs (instead of production Certs)")

	return cmd
}

func runRevoke(options *leOptions) (err error) {

	if options.staging {
		options.acmeserver = acmeStaging
	}

	accountsStorage := NewAccountsStorage(options)

	account, client := setup(options, accountsStorage)

	if account.Registration == nil {
		log.Fatalf("Account %s is not registered. Use 'run' to register a new account.\n", account.Email)
	}

	certsStorage := NewCertificatesStorage(options.path)
	certsStorage.CreateRootFolder()

	log.Infof("Trying to revoke certificate for domain %s", options.domain)

	certBytes, err := certsStorage.ReadFile(options.domain, ".crt")
	if err != nil {
		log.Fatalf("Error while revoking the certificate for domain %s\n\t%v", options.domain, err)
	}

	err = client.Certificate.Revoke(certBytes)
	if err != nil {
		log.Fatalf("Error while revoking the certificate for domain %s\n\t%v", options.domain, err)
	}

	log.Infof("Certificate was revoked.")

	certsStorage.CreateArchiveFolder()

	err = certsStorage.MoveToArchive(options.domain)
	if err != nil {
		return err
	}

	log.Infof("Certificate was archived for domain: %v", options.domain)

	return nil

}
