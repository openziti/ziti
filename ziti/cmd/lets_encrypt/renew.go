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
	"crypto/x509"
	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/spf13/cobra"
	"io"
	"time"
)

const (
	renewEnvAccountEmail = "LEGO_ACCOUNT_EMAIL"
	renewEnvCertDomain   = "LEGO_CERT_DOMAIN"
	renewEnvCertPath     = "LEGO_CERT_PATH"
	renewEnvCertKeyPath  = "LEGO_CERT_KEY_PATH"
)

// newListCmd creates a command object for the "controller list" command
func newRenewCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &leOptions{}

	cmd := &cobra.Command{
		Use:   "renew",
		Short: "Renew a Let's Encrypt certificate",
		Long:  "Renew a Let's Encrypt certificate",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runRenew(options)
			cmdhelper.CheckErr(err)
		},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().StringVarP(&options.domain, "domain", "d", "", "Domain for which Cert is being generated (e.g. me.example.com)")
	cmd.MarkFlagRequired("domain")
	cmd.Flags().StringVarP(&options.path, "path", "p", "", "Directory where data is stored")
	cmd.MarkFlagRequired("path")
	cmd.Flags().IntVarP(&options.days, "days", "", 14, "The number of days left on a certificate to renew it")
	cmd.Flags().BoolVarP(&options.reuseKey, "reuse-key", "r", true, "Used to indicate you want to reuse your current private key for the renewed certificate")
	cmd.Flags().StringVarP(&options.email, "email", "e", "openziti@netfoundry.io", "Email used for registration and recovery contact")
	options.keyType.Set("RSA4096") // set default
	cmd.Flags().VarP(&options.keyType, "keytype", "k", "Key type to use for private keys")
	cmd.Flags().StringVarP(&options.acmeserver, "acmeserver", "a", acmeProd, "ACME CA hostname")
	cmd.Flags().BoolVarP(&options.staging, "staging", "s", false, "Enable creation of 'staging' Certs (instead of production Certs)")

	return cmd
}

func runRenew(options *leOptions) (err error) {

	if options.staging {
		options.acmeserver = acmeStaging
	}

	accountsStorage := NewAccountsStorage(options)

	account, client := setup(options, accountsStorage)

	if account.Registration == nil {
		log.Fatalf("Account %s is not registered. Use 'run' to register a new account.\n", account.Email)
	}

	certsStorage := NewCertificatesStorage(options.path)

	meta := map[string]string{renewEnvAccountEmail: account.Email}

	return renewForDomains(options, client, certsStorage, meta)
}

func renewForDomains(options *leOptions, client *lego.Client, certsStorage *CertificatesStorage, meta map[string]string) error {
	domain := options.domain

	// load the cert resource from files.
	// We store the certificate, private key and metadata in different files
	// as web servers would not be able to work with a combined file.
	certificates, err := certsStorage.ReadCertificate(domain, ".crt")
	if err != nil {
		log.Fatalf("Error while loading the certificate for domain %s\n\t%v", domain, err)
	}

	cert := certificates[0]

	if !needRenewal(cert, domain, options.days) {
		return nil
	}

	// This is just meant to be informal for the user.
	timeLeft := cert.NotAfter.Sub(time.Now().UTC())
	log.Infof("[%s] acme: Trying renewal with %d hours remaining", domain, int(timeLeft.Hours()))

	certDomains := certcrypto.ExtractDomains(cert)

	var privateKey crypto.PrivateKey
	if options.reuseKey {
		keyBytes, errR := certsStorage.ReadFile(domain, ".key")
		if errR != nil {
			log.Fatalf("Error while loading the private key for domain %s\n\t%v", domain, errR)
		}

		privateKey, errR = certcrypto.ParsePEMPrivateKey(keyBytes)
		if errR != nil {
			return errR
		}
	}

	request := certificate.ObtainRequest{
		Domains:    merge(certDomains, domain),
		PrivateKey: privateKey,
	}
	certRes, err := client.Certificate.Obtain(request)
	if err != nil {
		log.Fatalf("%v", err)
	}

	certsStorage.SaveResource(certRes)

	return nil

}

func needRenewal(x509Cert *x509.Certificate, domain string, days int) bool {
	if x509Cert.IsCA {
		log.Fatalf("[%s] Certificate bundle starts with a CA certificate", domain)
	}

	if days >= 0 {
		notAfter := int(time.Until(x509Cert.NotAfter).Hours() / 24.0)
		if notAfter > days {
			log.Infof("[%s] The certificate expires in %d days, the number of days defined to perform the renewal is %d: no renewal.",
				domain, notAfter, days)
			return false
		}
	}

	return true
}

func merge(prevDomains []string, nextDomain string) []string {
	var found bool
	for _, prev := range prevDomains {
		if prev == nextDomain {
			found = true
			break
		}
	}
	if !found {
		prevDomains = append(prevDomains, nextDomain)
	}
	return prevDomains
}
