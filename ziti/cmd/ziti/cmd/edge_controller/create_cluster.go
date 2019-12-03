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

// newCreateClusterCmd creates the 'edge controller create cluster' command
func newCreateClusterCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &commonOptions{
		CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
	}

	cmd := &cobra.Command{
		Use:   "cluster <name>",
		Short: "creates a cluster managed by the Ziti Edge Controller",
		Long:  "creates a cluster managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateCluster(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)

	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")

	return cmd
}

// runCreateCluster create a new cluster on the Ziti Edge Controller
func runCreateCluster(o *commonOptions) error {

	serviceData := gabs.New()
	setJSONValue(serviceData, o.Args[0], "name")

	result, err := createEntityOfType("clusters", serviceData.String(), o)

	if err != nil {
		panic(err)
	}

	clusterId := result.S("data", "id").Data()

	if _, err := fmt.Fprintf(o.Out, "%v\n", clusterId); err != nil {
		panic(err)
	}

	return err
}
