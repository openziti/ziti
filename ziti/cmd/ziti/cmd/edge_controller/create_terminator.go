/*
	Copyright NetFoundry, Inc.

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

type createTerminatorOptions struct {
	commonOptions
	binding string
	tags    map[string]string
}

// newCreateTerminatorCmd creates the 'edge controller create Terminator local' command for the given entity type
func newCreateTerminatorCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createTerminatorOptions{
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
		Use:   "terminator service router address",
		Short: "creates a service terminator managed by the Ziti Edge Controller",
		Long:  "creates a service terminator managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateTerminator(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringToStringVarP(&options.tags, "tags", "t", nil, "Add tags to terminator definition")
	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	cmd.Flags().StringVar(&options.binding, "binding", "transport", "Add tags to terminator definition")

	return cmd
}

// runCreateTerminator implements the command to create a Terminator
func runCreateTerminator(o *createTerminatorOptions) (err error) {
	entityData := gabs.New()
	service, err := mapNameToID("services", o.Args[0])
	if err != nil {
		return err
	}

	router, err := mapNameToID("edge-routers", o.Args[1])
	if err != nil {
		router = o.Args[1] // might be a pure fabric router, id might not be UUID
	}

	setJSONValue(entityData, service, "service")
	setJSONValue(entityData, router, "router")
	setJSONValue(entityData, o.binding, "binding")
	setJSONValue(entityData, o.Args[2], "address")
	setJSONValue(entityData, o.tags, "tags")

	result, err := createEntityOfType("terminators", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	TerminatorId := result.S("data", "id").Data()

	if _, err = fmt.Fprintf(o.Out, "%v\n", TerminatorId); err != nil {
		panic(err)
	}

	return err
}
