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
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
	"io"
)

type appWanOptions struct {
	commonOptions
	identities []string
	services   []string
}

// newCreateAppWanCmd creates the 'edge controller create appwan' command
func newCreateAppWanCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &appWanOptions{
		commonOptions: commonOptions{
			CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "app-wan <name>",
		Short: "creates an AppWan managed by the Ziti Edge Controller",
		Long:  "creates an AppWan managed by the Ziti Edge Controller",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateAppWan(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	cmd.Flags().SetInterspersed(true) // allow interspersing positional args and flags
	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	cmd.Flags().StringSliceVarP(&options.identities, "identities", "i", nil, "Identities to add to the AppWan")
	cmd.Flags().StringSliceVarP(&options.services, "services", "s", nil, "Services to add to the AppWan")

	return cmd
}

func runCreateAppWan(o *appWanOptions) error {

	serviceData := gabs.New()

	identityIds, err := mapNamesToIDs("identities", o.identities...)
	if err != nil {
		return err
	}

	serviceIds, err := mapNamesToIDs("services", o.services...)
	if err != nil {
		return err
	}

	setJSONValue(serviceData, o.Args[0], "name")
	setJSONValue(serviceData, identityIds, "identities")
	setJSONValue(serviceData, serviceIds, "services")

	if _, err := fmt.Fprintf(o.Out, "%v\n", serviceData.String()); err != nil {
		panic(err)
	}

	result, err := createEntityOfType("app-wans", serviceData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	appWanId := result.S("data", "id").Data()

	if _, err := fmt.Fprintf(o.Out, "%v\n", appWanId); err != nil {
		panic(err)
	}

	return err
}
