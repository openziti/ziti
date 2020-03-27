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
	"encoding/json"
	"fmt"
	"io"

	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

type createConfigOptions struct {
	commonOptions
}

// newCreateConfigCmd creates the 'edge controller create service-policy' command
func newCreateConfigCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createConfigOptions{
		commonOptions: commonOptions{
			CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "config <name> <type> <JSON configuration data>",
		Short: "creates a config managed by the Ziti Edge Controller",
		Long:  "creates a config managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateConfig(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")

	return cmd
}

// runCreateConfig create a new config on the Ziti Edge Controller
func runCreateConfig(o *createConfigOptions) error {
	dataMap := map[string]interface{}{}
	if err := json.Unmarshal([]byte(o.Args[2]), &dataMap); err != nil {
		fmt.Printf("Attempted to parse: %v\n", o.Args[1])
		fmt.Printf("Failing parsing JSON: %+v\n", err)
		return errors.Errorf("unable to parse data as json: %v", err)
	}

	configTypeId, err := mapNameToID("config-types", o.Args[1])
	if err != nil {
		return err
	}

	entityData := gabs.New()
	setJSONValue(entityData, o.Args[0], "name")
	setJSONValue(entityData, configTypeId, "type")
	setJSONValue(entityData, dataMap, "data")
	result, err := createEntityOfType("configs", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	configId := result.S("data", "id").Data()

	if _, err := fmt.Fprintf(o.Out, "%v\n", configId); err != nil {
		panic(err)
	}

	return err
}
