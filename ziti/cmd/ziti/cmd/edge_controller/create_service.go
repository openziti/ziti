/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
)

type createServiceOptions struct {
	commonOptions
	hostedService  bool
	tags           map[string]string
	roleAttributes []string
	configs        []string
}

// newCreateServiceCmd creates the 'edge controller create service local' command for the given entity type
func newCreateServiceCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createServiceOptions{
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
		Use:   "service <name> [egress node]? [egress endpoint uri]?",
		Short: "creates a service managed by the Ziti Edge Controller",
		Long:  "creates a service managed by the Ziti Edge Controller",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateService(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringToStringVarP(&options.tags, "tags", "t", nil, "Add tags to service definition")
	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	cmd.Flags().BoolVar(&options.hostedService, "hosted", false, "Indicates that this is a hosted service")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil, "Role attributes of the new service")
	cmd.Flags().StringSliceVarP(&options.configs, "configs", "c", nil, "Configuration id or names to be associated with the new service")

	return cmd
}

// runCreateNativeService implements the command to create a service
func runCreateService(o *createServiceOptions) (err error) {
	entityData := gabs.New()
	setJSONValue(entityData, o.Args[0], "name")
	setJSONValue(entityData, o.roleAttributes, "roleAttributes")
	setJSONValue(entityData, o.configs, "configs")

	if o.hostedService || len(o.Args) == 1 {
		setJSONValue(entityData, "unclaimed", "egressRouter")
		setJSONValue(entityData, "hosted:unclaimed", "endpointAddress")
	} else {
		if len(o.Args) < 3 {
			return errors.Errorf("if --hosted is not set, you must provide egress router and endpoint address")
		}
		setJSONValue(entityData, o.Args[1], "egressRouter")
		setJSONValue(entityData, o.Args[2], "endpointAddress")
	}

	setJSONValue(entityData, o.tags, "tags")

	result, err := createEntityOfType("services", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	serviceId := result.S("data", "id").Data()

	if _, err = fmt.Fprintf(o.Out, "%v\n", serviceId); err != nil {
		panic(err)
	}

	return err
}
