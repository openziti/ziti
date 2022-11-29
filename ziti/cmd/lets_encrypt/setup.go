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
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/openziti/ziti/ziti/internal/log"
	"io/ioutil"
	"os"
	"time"

	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

func setup(options *leOptions, accountsStorage *AccountsStorage) (*Account, *lego.Client) {

	privateKey := accountsStorage.GetPrivateKey(options.keyType.Get())

	var account *Account
	if accountsStorage.ExistsAccountFilePath() {
		account = accountsStorage.LoadAccount(options, privateKey)
	} else {
		account = &Account{Email: accountsStorage.GetUserID(), key: privateKey}
	}

	client := newClient(options, account)

	err := client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", options.port))
	if err != nil {
		log.Fatalf("%v", err)
	}

	return account, client
}

func newClient(options *leOptions, acc registration.User) *lego.Client {
	config := lego.NewConfig(acc)
	config.CADirURL = options.acmeserver

	config.Certificate = lego.CertificateConfig{
		KeyType: options.keyType.Get(),
		Timeout: time.Duration(30) * time.Second, // Only used when obtaining certificates
	}
	config.UserAgent = fmt.Sprintf("zitii-cli")

	client, err := lego.NewClient(config)
	if err != nil {
		log.Fatalf("Could not create client: %v", err)
	}

	return client
}

func getEmail(options *leOptions) string {
	email := options.email
	if len(email) == 0 {
		log.Fatal("You have to pass an account (email address) to the program using --email or -m")
	}
	return email
}

func createNonExistingFolder(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0o700)
	} else if err != nil {
		return err
	}
	return nil
}

func readCSRFile(filename string) (*x509.CertificateRequest, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	raw := bytes

	// see if we can find a PEM-encoded CSR
	var p *pem.Block
	rest := bytes
	for {
		// decode a PEM block
		p, rest = pem.Decode(rest)

		// did we fail?
		if p == nil {
			break
		}

		// did we get a CSR?
		if p.Type == "CERTIFICATE REQUEST" {
			raw = p.Bytes
		}
	}

	// no PEM-encoded CSR
	// assume we were given a DER-encoded ASN.1 CSR
	// (if this assumption is wrong, parsing these bytes will fail)
	return x509.ParseCertificateRequest(raw)
}
