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
	"crypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/spf13/cobra"
	"io"
)

const acmeStaging string = "https://acme-staging-v02.api.letsencrypt.org/directory"
const acmeProd string = "https://acme-v02.api.letsencrypt.org/directory"

const rootPathWarningMessage = `!!!! HEADS UP !!!!`

// We need a user or account type that implements acme.User
type AcmeUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *AcmeUser) GetEmail() string {
	return u.Email
}
func (u AcmeUser) GetRegistration() *registration.Resource {
	return u.Registration
}
func (u *AcmeUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// newCreateCmd creates the 'edge controller create ca local' command for the given entity type
func newCreateCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &leOptions{}

	cmd := &cobra.Command{
		Use:   "create -d <domain> -p <path-to-where-data-is-saved>",
		Short: "Register a Let's Encrypt account, then create and install a certificate",
		Long:  "Register a Let's Encrypt account, then create and install a certificate",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreate(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().StringVarP(&options.domain, "domain", "d", "", "Domain for which Cert is being generated (e.g. me.example.com)")
	cmd.MarkFlagRequired("domain")
	cmd.Flags().BoolVarP(&options.staging, "staging", "s", false, "Enable creation of 'staging' Certs (instead of production Certs)")
	cmd.Flags().StringVarP(&options.acmeserver, "acmeserver", "a", acmeProd, "ACME CA hostname")
	cmd.Flags().StringVarP(&options.email, "email", "e", "openziti@netfoundry.io", "Email used for registration and recovery contact")
	options.keyType.Set("RSA4096") // set default
	cmd.Flags().VarP(&options.keyType, "keytype", "k", "Key type to use for private keys")
	cmd.Flags().StringVarP(&options.path, "path", "p", "", "Directory to use for storing the data")
	cmd.MarkFlagRequired("path")
	cmd.Flags().StringVarP(&options.port, "port", "o", "80", "Port to listen on for HTTP based ACME challenges")
	cmd.Flags().StringVarP(&options.csr, "csr", "", "", "Certificate Signing Request filename, if an external CSR is to be used")

	return cmd
}

func runCreate(options *leOptions) (err error) {

	if options.staging {
		options.acmeserver = acmeStaging
	}

	accountsStorage := NewAccountsStorage(options)

	account, client := setup(options, accountsStorage)

	if account.Registration == nil {
		reg, err := register(options, client)
		if err != nil {
			log.Fatalf("Could not complete registration\n\t%v", err)
		}

		account.Registration = reg
		if err = accountsStorage.Save(account); err != nil {
			log.Fatalf("%v", err)
		}
	}

	request := certificate.ObtainRequest{
		Domains: []string{options.domain},
		Bundle:  true,
	}

	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatalf("%v", err)
	}

	certsStorage := NewCertificatesStorage(options.path)

	certsStorage.CreateRootFolder()

	certsStorage.SaveResource(certificates)

	return nil
}

func register(options *leOptions, client *lego.Client) (*registration.Resource, error) {

	return client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
}
