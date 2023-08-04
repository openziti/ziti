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
	"github.com/openziti/ziti/ziti/internal/log"
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
	config.UserAgent = "ziti-cli"

	client, err := lego.NewClient(config)
	if err != nil {
		log.Fatalf("Could not create client: %v", err)
	}

	return client
}

func createNonExistingFolder(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0o700)
	} else if err != nil {
		return err
	}
	return nil
}
