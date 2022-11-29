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

package fabric

import (
	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
)

type createServiceOptions struct {
	api.Options
	terminatorStrategy string
	tags               map[string]string
}

// newCreateServiceCmd creates the 'fabric create service' command for the given entity type
func newCreateServiceCmd(p common.OptionsProvider) *cobra.Command {
	options := &createServiceOptions{
		Options: api.Options{
			CommonOptions: p(),
		},
		tags: make(map[string]string),
	}

	cmd := &cobra.Command{
		Use:        "service <name>",
		Short:      "creates a service managed by the Ziti Controller",
		Args:       cobra.MinimumNArgs(1),
		RunE:       options.createService,
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringToStringVarP(&options.tags, "tags", "t", nil, "Add tags to service definition")
	cmd.Flags().StringVar(&options.terminatorStrategy, "terminator-strategy", "", "Specifies the terminator strategy for the service")
	options.AddCommonFlags(cmd)

	return cmd
}

// createService implements the command to create a service
func (o *createServiceOptions) createService(_ *cobra.Command, args []string) (err error) {
	o.Args = args
	entityData := gabs.New()
	api.SetJSONValue(entityData, args[0], "name")
	if o.terminatorStrategy != "" {
		api.SetJSONValue(entityData, o.terminatorStrategy, "terminatorStrategy")
	}

	api.SetJSONValue(entityData, o.tags, "tags")

	result, err := createEntityOfType("services", entityData.String(), &o.Options)
	return o.LogCreateResult("service", result, err)
}
