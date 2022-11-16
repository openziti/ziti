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
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type updateLinkOptions struct {
	api.Options
	down       bool
	staticCost uint32
}

func newUpdateLinkCmd(p common.OptionsProvider) *cobra.Command {
	options := &updateLinkOptions{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "link <idOrName>",
		Short: "updates a link managed by the Ziti Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateLink(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVar(&options.down, "down", true, "Set link up or down")
	cmd.Flags().Uint32Var(&options.staticCost, "static-cost", 0, "Specifies the static cost of the link")
	options.AddCommonFlags(cmd)

	return cmd
}

// runUpdateLink update a new link on the Ziti Edge Controller
func runUpdateLink(o *updateLinkOptions) error {
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("down") {
		api.SetJSONValue(entityData, o.down, "down")
		change = true
	}

	if o.Cmd.Flags().Changed("static-cost") {
		api.SetJSONValue(entityData, o.staticCost, "staticCost")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	_, err := patchEntityOfType(fmt.Sprintf("links/%v", o.Args[0]), entityData.String(), &o.Options)
	return err
}
