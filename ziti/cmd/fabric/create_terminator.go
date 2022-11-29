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
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/edge/router/xgress_edge_transport"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"math"
)

type createTerminatorOptions struct {
	api.Options
	binding    string
	cost       int32
	precedence string
	instanceId string
}

// newCreateTerminatorCmd creates the 'fabric create terminator' command
func newCreateTerminatorCmd(p common.OptionsProvider) *cobra.Command {
	options := &createTerminatorOptions{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "terminator service router address",
		Short: "creates a service terminator managed by the Ziti Controller",
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
	cmd.Flags().StringVar(&options.binding, "binding", xgress_edge_transport.BindingName, "Set the terminator binding")
	cmd.Flags().Int32VarP(&options.cost, "cost", "c", 0, "Set the terminator cost")
	cmd.Flags().StringVarP(&options.precedence, "precedence", "p", "", "Set the terminator precedence ('default', 'required' or 'failed')")
	cmd.Flags().StringVar(&options.instanceId, "instance-id", "", "Set the terminator instance-id")
	options.AddCommonFlags(cmd)

	return cmd
}

// runCreateTerminator implements the command to create a Terminator
func runCreateTerminator(o *createTerminatorOptions) (err error) {
	entityData := gabs.New()
	service, err := api.MapNameToID(util.FabricAPI, "services", &o.Options, o.Args[0])
	if err != nil {
		return err
	}

	router, err := api.MapNameToID(util.FabricAPI, "routers", &o.Options, o.Args[1])
	if err != nil {
		return err
	}

	api.SetJSONValue(entityData, service, "service")
	api.SetJSONValue(entityData, router, "router")
	api.SetJSONValue(entityData, o.binding, "binding")
	api.SetJSONValue(entityData, o.Args[2], "address")
	api.SetJSONValue(entityData, o.instanceId, "instanceId")
	if o.cost > 0 {
		if o.cost > math.MaxUint16 {
			if _, err = fmt.Fprintf(o.Out, "Invalid cost %v. Must be positive number less than or equal to %v\n", o.cost, math.MaxUint16); err != nil {
				panic(err)
			}
			return
		}
		api.SetJSONValue(entityData, o.cost, "cost")
	}
	if o.precedence != "" {
		validValues := []string{"default", "required", "failed"}
		if !stringz.Contains(validValues, o.precedence) {
			if _, err = fmt.Fprintf(o.Out, "Invalid precedence %v. Must be one of %+v\n", o.precedence, validValues); err != nil {
				panic(err)
			}
			return
		}
		api.SetJSONValue(entityData, o.precedence, "precedence")
	}

	result, err := createEntityOfType("terminators", entityData.String(), &o.Options)
	return o.LogCreateResult("terminator", result, err)
}
