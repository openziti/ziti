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

package edge

import (
	"encoding/json"
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"io"
	"io/ioutil"

	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type updateConfigOptions struct {
	api.Options
	name     string
	data     string
	jsonFile string
	tags     map[string]string
}

// newUpdateConfigCmd updates the 'edge controller update service-policy' command
func newUpdateConfigCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updateConfigOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "config <idOrName>",
		Short: "updates a config managed by the Ziti Edge Controller",
		Long:  "updates a config managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateConfig(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name of the config")
	cmd.Flags().StringVarP(&options.data, "data", "d", "", "Set the data of the config")
	cmd.Flags().StringVarP(&options.jsonFile, "json-file", "f", "", "Read config JSON from a file instead of the command line")
	cmd.Flags().StringToStringVar(&options.tags, "tags", nil, "Custom management tags")

	options.AddCommonFlags(cmd)

	return cmd
}

// runUpdateConfig update a new config on the Ziti Edge Controller
func runUpdateConfig(o *updateConfigOptions) error {
	id, err := mapNameToID("configs", o.Args[0], o.Options)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		api.SetJSONValue(entityData, o.name, "name")
		change = true
	}

	var jsonBytes []byte

	if o.Cmd.Flags().Changed("data") {
		jsonBytes = []byte(o.data)
	}

	if o.Cmd.Flags().Changed("json-file") {
		if o.Cmd.Flags().Changed("data") {
			return errors.New("only one of --data and --json-file is allowed")
		}
		var err error
		if jsonBytes, err = ioutil.ReadFile(o.jsonFile); err != nil {
			return fmt.Errorf("failed to read config json file %v: %w", o.jsonFile, err)
		}
	}

	if o.Cmd.Flags().Changed("tags") {
		api.SetJSONValue(entityData, o.tags, "tags")
		change = true
	}

	if len(jsonBytes) > 0 {
		dataMap := map[string]interface{}{}
		if err := json.Unmarshal(jsonBytes, &dataMap); err != nil {
			fmt.Printf("Attempted to parse: %v\n", string(jsonBytes))
			fmt.Printf("Failing parsing JSON: %+v\n", err)
			return errors.Errorf("unable to parse data as json: %v", err)
		}
		api.SetJSONValue(entityData, dataMap, "data")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	_, err = patchEntityOfType(fmt.Sprintf("configs/%v", id), entityData.String(), &o.Options)

	if err != nil {
		panic(err)
	}

	return err
}
