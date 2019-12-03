/*
	Copyright 2019 Netfoundry, Inc.

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

package edge_controller

import (
	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/util"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
	"io"
)

// newListCmd creates a command object for the "controller list" command
func newCreateCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "creates various entities managed by the Ziti Edge Controller",
		Long:  "Creates various entities managed by the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newCreateAppWanCmd(f, out, errOut))
	cmd.AddCommand(newCreateClusterCmd(f, out, errOut))
	cmd.AddCommand(newCreateEdgeRouterCmd(f, out, errOut))
	cmd.AddCommand(newCreateIdentityCmd(f, out, errOut))
	cmd.AddCommand(newCreateServiceCmd(f, out, errOut))
	cmd.AddCommand(newCreateAuthenticatorCmd(f, out, errOut))
	cmd.AddCommand(newCreateCaCmd(f, out, errOut))

	return cmd
}

// createEntityOfType create an entity of the given type on the Ziti Edge Controller
func createEntityOfType(entityType string, body string, options *commonOptions) (*gabs.Container, error) {

	session := &session{}
	err := session.Load()

	if err != nil {
		return nil, err
	}

	if session.Host == "" {
		return nil, fmt.Errorf("host not specififed in cli config file. Exiting")
	}

	jsonParsed, err := util.EdgeControllerCreate(session.Host, session.Cert, session.Token, entityType, body, options.Out, options.OutputJSONResponse)

	if err != nil {
		panic(err)
	}

	return jsonParsed, nil
}
