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
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"io"
)

// newCreateCmd creates a command object for the "list" command
func newShowCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "displays various entities managed by the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	showCmd.AddCommand(newShowConfigTypeAction(out, errOut))
	showCmd.AddCommand(newShowConfigAction(out, errOut))
	return showCmd
}

func newShowConfigAction(out io.Writer, errOut io.Writer) *cobra.Command {
	action := &showConfigAction{
		Options: api.Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	showConfigDefCmd := &cobra.Command{
		Use:   "config <id or name>",
		Short: "displays the JSON config definition for a given config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.run(cmd, args)
		},
	}

	action.AddCommonFlags(showConfigDefCmd)

	return showConfigDefCmd
}

type showConfigAction struct {
	api.Options
}

func (self *showConfigAction) run(_ *cobra.Command, args []string) error {
	id, err := mapNameToID("configs", args[0], self.Options)
	if err != nil {
		return err
	}

	jsonVal, err := util.ControllerDetailEntity(util.EdgeAPI, "configs", id, self.OutputJSONResponse, self.Out, self.Timeout, self.Verbose)
	if err != nil {
		return err
	}

	if self.OutputJSONResponse {
		return nil
	}

	schema := jsonVal.Path("data.data")
	formattedData, err := json.MarshalIndent(schema.Data(), "", "    ")
	if err != nil {
		return err
	}

	fmt.Println(string(formattedData))

	return nil
}

func newShowConfigTypeAction(out io.Writer, errOut io.Writer) *cobra.Command {
	action := &showConfigTypeAction{
		Options: api.Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}

	showConfigTypeSchemaCmd := &cobra.Command{
		Use:   "config-type <id or name>",
		Short: "displays the JSON schema for a given config type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			action.Cmd = cmd
			action.Args = args
			return action.run(cmd, args)
		},
	}

	action.AddCommonFlags(showConfigTypeSchemaCmd)

	return showConfigTypeSchemaCmd
}

type showConfigTypeAction struct {
	api.Options
}

func (self *showConfigTypeAction) run(_ *cobra.Command, args []string) error {
	id, err := mapNameToID("config-types", args[0], self.Options)
	if err != nil {
		return err
	}

	jsonVal, err := util.ControllerDetailEntity(util.EdgeAPI, "config-types", id, self.OutputJSONResponse, self.Out, self.Timeout, self.Verbose)
	if err != nil {
		return err
	}

	if self.OutputJSONResponse {
		return nil
	}

	schema := jsonVal.Path("data.schema")
	formattedData, err := json.MarshalIndent(schema.Data(), "", "    ")
	if err != nil {
		return err
	}

	fmt.Println(string(formattedData))

	return nil
}
