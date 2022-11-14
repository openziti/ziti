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

package fabric

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
)

type addIdentityOptions struct {
	common.CommonOptions
	caCert     string
	clientCert string
	clientKey  string
	readOnly   bool
}

func newAddIdentityCmd(p common.OptionsProvider) *cobra.Command {
	options := &addIdentityOptions{
		CommonOptions: p(),
	}

	cmd := &cobra.Command{
		Use:   "add-identity <server-url>",
		Short: "adds a fabric identity for connecting to the Ziti Controller",
		Args:  cobra.ExactArgs(1),
		RunE:  options.run,
	}

	cmd.Flags().StringVarP(&common.CliIdentity, "cli-identity", "i", "", "Specify the saved identity you want the CLI to use when connect to the controller with")
	cmd.Flags().StringVar(&options.caCert, "ca-cert", "", "additional root certificates used by the Ziti Controller")
	cmd.Flags().StringVar(&options.clientCert, "client-cert", "", "client certificate used to authenticate to the Ziti Controller")
	cmd.Flags().StringVar(&options.clientKey, "client-key", "", "client certificate key used to authenticate to the Ziti Controller")
	cmd.Flags().BoolVar(&options.readOnly, "read-only", false, "marks this login as read-only. Note: this is not a guarantee that nothing can be changed on the server. Care should still be taken!")

	for _, required := range []string{"ca-cert", "client-cert", "client-key"} {
		if err := cmd.MarkFlagRequired(required); err != nil {
			panic(err)
		}
	}

	return cmd
}

func (o *addIdentityOptions) run(_ *cobra.Command, args []string) error {
	config, configFile, err := util.LoadRestClientConfig()
	if err != nil {
		return err
	}

	id := config.GetIdentity()

	loginIdentity := &util.RestClientFabricIdentity{
		Url:        args[0],
		CaCert:     o.caCert,
		ClientCert: o.clientCert,
		ClientKey:  o.clientKey,
		ReadOnly:   o.readOnly,
	}

	o.Printf("Saving identity '%v' to %v\n", id, configFile)
	config.FabricIdentities[id] = loginIdentity

	err = util.PersistRestClientConfig(config)

	return nil
}
