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
	"errors"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/foundation/util/stringz"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	errors2 "github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
	"math"
)

type updateTerminatorOptions struct {
	commonOptions
	router     string
	address    string
	binding    string
	cost       int32
	precedence string
}

func newUpdateTerminatorCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updateTerminatorOptions{
		commonOptions: commonOptions{
			CommonOptions: common.CommonOptions{
				Factory: f,
				Out:     out,
				Err:     errOut,
			},
		},
	}

	cmd := &cobra.Command{
		Use:   "terminator <id>",
		Short: "updates a service terminator",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateTerminator(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVar(&options.router, "router", "", "Set the terminator router")
	cmd.Flags().StringVar(&options.address, "address", "", "Set the terminator address")
	cmd.Flags().StringVar(&options.binding, "binding", "", "Set the terminator binding")
	cmd.Flags().Int32VarP(&options.cost, "cost", "c", 0, "Set the terminator cost")
	cmd.Flags().StringVarP(&options.precedence, "precedence", "p", "", "Set the terminator precedence ('default', 'required' or 'failed')")
	options.AddCommonFlags(cmd)

	return cmd
}

// runUpdateTerminator implements the command to update a Terminator
func runUpdateTerminator(o *updateTerminatorOptions) (err error) {
	entityData := gabs.New()

	router, err := mapNameToID("edge-routers", o.router, o.commonOptions)
	if err != nil {
		router = o.router // might be a pure fabric router, id might not be UUID
	}

	change := false
	if o.Cmd.Flags().Changed("router") {
		setJSONValue(entityData, router, "router")
		change = true
	}

	if o.Cmd.Flags().Changed("binding") {
		setJSONValue(entityData, o.binding, "binding")
		change = true
	}

	if o.Cmd.Flags().Changed("address") {
		setJSONValue(entityData, o.address, "address")
		change = true
	}

	if o.Cmd.Flags().Changed("cost") {
		if o.cost > math.MaxUint16 {
			return errors2.Errorf("Invalid cost %v. Must be positive number less than or equal to %v", o.cost, math.MaxUint16)
		}
		setJSONValue(entityData, o.cost, "cost")
		change = true
	}

	if o.Cmd.Flags().Changed("precedence") {
		validValues := []string{"default", "required", "failed"}
		if !stringz.Contains(validValues, o.precedence) {
			return errors2.Errorf("Invalid precedence %v. Must be one of %+v", o.precedence, validValues)
		}
		setJSONValue(entityData, o.precedence, "precedence")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	_, err = patchEntityOfType(fmt.Sprintf("terminators/%v", o.Args[0]), entityData.String(), &o.commonOptions)
	return err
}
