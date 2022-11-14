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
	"encoding/json"
	"fmt"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"io"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/spf13/cobra"
)

// newListCmd creates a command object for the "controller list" command
func newListCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &leOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Display Let's Encrypt certificates and accounts information",
		Long:  "Display Let's Encrypt certificates and accounts information",
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runList(options)
			cmdhelper.CheckErr(err)
		},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().BoolVarP(&options.accounts, "accounts", "a", false, "Display Account info")
	cmd.Flags().BoolVarP(&options.names, "names", "n", false, "Display Names info")
	cmd.Flags().StringVarP(&options.path, "path", "p", "", "Directory where data is stored")
	cmd.MarkFlagRequired("path")

	return cmd
}

func runList(options *leOptions) (err error) {
	if options.accounts && !options.names {
		if err := listAccount(options); err != nil {
			return err
		}
	}

	return listCertificates(options)
}

func listAccount(options *leOptions) error {

	accountsStorage := NewAccountsStorage(options)

	matches, err := filepath.Glob(filepath.Join(accountsStorage.GetRootPath(), "*", "*", "*.json"))
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		fmt.Println("No accounts found.")
		return nil
	}

	fmt.Println("Found the following accounts:")
	for _, filename := range matches {
		data, err := ioutil.ReadFile(filename)
		if err != nil {
			return err
		}

		var account Account
		err = json.Unmarshal(data, &account)
		if err != nil {
			return err
		}

		uri, err := url.Parse(account.Registration.URI)
		if err != nil {
			return err
		}

		fmt.Println("  Email:", account.Email)
		fmt.Println("  Server:", uri.Host)
		fmt.Println("  Path:", filepath.Dir(filename))
		fmt.Println()
	}

	return nil
}

func listCertificates(options *leOptions) error {
	certsStorage := NewCertificatesStorage(options.path)

	matches, err := filepath.Glob(filepath.Join(certsStorage.GetRootPath(), "*.crt"))
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		if !options.names {
			fmt.Println("No certificates found.")
		}
		return nil
	}

	if !options.names {
		fmt.Println("Found the following certs:")
	}

	for _, filename := range matches {
		if strings.HasSuffix(filename, ".issuer.crt") {
			continue
		}

		data, err := ioutil.ReadFile(filename)
		if err != nil {
			return err
		}

		pCert, err := certcrypto.ParsePEMCertificate(data)
		if err != nil {
			return err
		}

		if options.names {
			fmt.Println(pCert.Subject.CommonName)
		} else {
			fmt.Println("  Certificate Name:", pCert.Subject.CommonName)
			fmt.Println("    Domains:", strings.Join(pCert.DNSNames, ", "))
			fmt.Println("    Expiry Date:", pCert.NotAfter)
			fmt.Println("    Certificate Path:", filename)
			fmt.Println()
		}
	}

	return nil
}
