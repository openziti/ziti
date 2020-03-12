/*
	Copyright 2020 NetFoundry, Inc.

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
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
)

type createEndpointOptions struct {
	commonOptions
	binding string
	tags    map[string]string
}

// newCreateEndpointCmd creates the 'edge controller create Endpoint local' command for the given entity type
func newCreateEndpointCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createEndpointOptions{
		commonOptions: commonOptions{
			CommonOptions: common.CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
		tags: make(map[string]string),
	}

	cmd := &cobra.Command{
		Use:   "endpoint <name> service router address",
		Short: "creates an endpoint managed by the Ziti Edge Controller",
		Long:  "creates an endpoint managed by the Ziti Edge Controller",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateEndpoint(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringToStringVarP(&options.tags, "tags", "t", nil, "Add tags to endpoint definition")
	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	cmd.Flags().StringVar(&options.binding, "binding", "transport", "Add tags to endpoint definition")

	return cmd
}

// runCreateEndpoint implements the command to create a Endpoint
func runCreateEndpoint(o *createEndpointOptions) (err error) {
	entityData := gabs.New()
	setJSONValue(entityData, o.Args[0], "service")
	setJSONValue(entityData, o.Args[1], "router")
	setJSONValue(entityData, o.binding, "binding")
	setJSONValue(entityData, o.Args[2], "address")
	setJSONValue(entityData, o.tags, "tags")

	result, err := createEntityOfType("endpoints", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	EndpointId := result.S("data", "id").Data()

	if _, err = fmt.Fprintf(o.Out, "%v\n", EndpointId); err != nil {
		panic(err)
	}

	return err
}
