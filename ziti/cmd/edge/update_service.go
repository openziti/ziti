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
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"io"

	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type updateServiceOptions struct {
	api.Options
	name               string
	terminatorStrategy string
	roleAttributes     []string
	encryption         encryptionVar
	configs            []string
	tags               map[string]string
}

func newUpdateServiceCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updateServiceOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "service <idOrName>",
		Short: "updates a service managed by the Ziti Edge Controller",
		Long:  "updates a service managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateService(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name of the service")
	cmd.Flags().StringVar(&options.terminatorStrategy, "terminator-strategy", "", "Specifies the terminator strategy for the service")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil,
		"Set role attributes of the service. Use --role-attributes '' to set an empty list")
	options.encryption.Set("ON")
	cmd.Flags().VarP(&options.encryption, "encryption", "e", "Controls end-to-end encryption for the service")
	cmd.Flags().StringSliceVarP(&options.configs, "configs", "c", nil, "Configuration id or names to be associated with the new service")
	cmd.Flags().StringToStringVar(&options.tags, "tags", nil, "Custom management tags")

	options.AddCommonFlags(cmd)

	return cmd
}

// runUpdateService update a new service on the Ziti Edge Controller
func runUpdateService(o *updateServiceOptions) error {
	id, err := mapNameToID("services", o.Args[0], o.Options)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		api.SetJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("role-attributes") {
		api.SetJSONValue(entityData, o.roleAttributes, "roleAttributes")
		change = true
	}

	if o.Cmd.Flags().Changed("terminator-strategy") {
		api.SetJSONValue(entityData, o.terminatorStrategy, "terminatorStrategy")
		change = true
	}

	if o.Cmd.Flags().Changed("encryption") {
		api.SetJSONValue(entityData, o.encryption.Get(), "encryptionRequired")
		change = true
	}

	if o.Cmd.Flags().Changed("configs") {
		configs, err := mapNamesToIDs("configs", o.Options, o.configs...)
		if err != nil {
			return err
		}
		api.SetJSONValue(entityData, configs, "configs")
		change = true
	}

	if o.Cmd.Flags().Changed("tags") {
		api.SetJSONValue(entityData, o.tags, "tags")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	_, err = patchEntityOfType(fmt.Sprintf("services/%v", id), entityData.String(), &o.Options)
	return err
}
