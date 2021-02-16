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

package lets_encrypt

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	// "github.com/go-acme/lego/v4/challenge/tlsalpn01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
	"log"
)

type createOptions struct {
	leOptions
	staging    bool
	domain     string
	acmeserver string
	email      string
	keyType    KeyTypeVar
	path       string
	http       bool
	port       string
}

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
func newCreateCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createOptions{
		leOptions: leOptions{},
	}

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
	cmd.Flags().BoolVarP(&options.staging, "prod", "", false, "Enable creation of 'production' Certs (instead of staging Certs)")
	cmd.Flags().StringVarP(&options.acmeserver, "acmeserver", "a", "https://acme-staging-v02.api.letsencrypt.org/directory", "ACME CA hostname")
	cmd.Flags().StringVarP(&options.email, "email", "e", "openziti@netfoundry.io", "Email used for registration and recovery contact")
	options.keyType.Set("RSA4096") // set default
	cmd.Flags().VarP(&options.keyType, "keytype", "k", "Key type to use for private keys")
	cmd.Flags().StringVarP(&options.path, "path", "p", "", "Directory to use for storing the data")
	cmd.MarkFlagRequired("path")
	cmd.Flags().StringVarP(&options.port, "port", "o", "80", "Port to listen on for HTTP based ACME challenges")

	return cmd
}

func runCreate(options *createOptions) (err error) {

	// Create a user. New accounts need an email and private key to start.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	acmeUser := AcmeUser{
		Email: options.email,
		key:   privateKey,
	}

	config := lego.NewConfig(&acmeUser)

	config.CADirURL = options.acmeserver
	config.Certificate.KeyType = options.keyType.Get()

	client, err := lego.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}

	err = client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", options.port))
	if err != nil {
		log.Fatal(err)
	}
	// err = client.Challenge.SetTLSALPN01Provider(tlsalpn01.NewProviderServer("", "443"))
	// if err != nil {
	// 	log.Fatal(err)
	// }

	reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		log.Fatal(err)
	}
	acmeUser.Registration = reg

	request := certificate.ObtainRequest{
		Domains: []string{options.domain},
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatal(err)
	}

	certsStorage := NewCertificatesStorage(options.path)

	certsStorage.CreateRootFolder()

	certsStorage.SaveResource(certificates)

	return nil
}
