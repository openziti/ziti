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
	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
)

type createServiceOptions struct {
	api.Options
	terminatorStrategy string
	tags               map[string]string
	roleAttributes     []string
	configs            []string
	requireEncryption  bool
	encryption         encryptionVar
}

// newCreateServiceCmd creates the 'edge controller create service local' command for the given entity type
func newCreateServiceCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createServiceOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
		tags: make(map[string]string),
	}

	cmd := &cobra.Command{
		Use:   "service <name>",
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
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil, "Role attributes of the new service")
	cmd.Flags().StringSliceVarP(&options.configs, "configs", "c", nil, "Configuration id or names to be associated with the new service")
	cmd.Flags().StringVar(&options.terminatorStrategy, "terminator-strategy", "", "Specifies the terminator strategy for the service")
	options.encryption.Set("ON")
	cmd.Flags().VarP(&options.encryption, "encryption", "e", "Controls end-to-end encryption for the service")
	options.AddCommonFlags(cmd)

	return cmd
}

// runCreateService implements the command to create a service
func runCreateService(o *createServiceOptions) (err error) {
	configs, err := mapNamesToIDs("configs", o.Options, o.configs...)
	if err != nil {
		return err
	}

	entityData := gabs.New()
	api.SetJSONValue(entityData, o.Args[0], "name")
	if o.terminatorStrategy != "" {
		api.SetJSONValue(entityData, o.terminatorStrategy, "terminatorStrategy")
	}

	api.SetJSONValue(entityData, o.encryption.Get(), "encryptionRequired")

	api.SetJSONValue(entityData, o.roleAttributes, "roleAttributes")
	api.SetJSONValue(entityData, configs, "configs")
	api.SetJSONValue(entityData, o.tags, "tags")

	result, err := CreateEntityOfType("services", entityData.String(), &o.Options)
	return o.LogCreateResult("service", result, err)
}
