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

	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type updateRouterOptions struct {
	api.Options
	name              string
	fingerprint       string
	cost              uint16
	noTraversal       bool
	disabled          bool
	ctrlChanListeners []string
	tags              map[string]string
}

func newUpdateRouterCmd(p common.OptionsProvider) *cobra.Command {
	options := &updateRouterOptions{
		Options: api.Options{CommonOptions: p()},
	}

	cmd := &cobra.Command{
		Use:   "router <idOrName>",
		Short: "updates a router managed by the Ziti Controller",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return runUpdateRouter(options)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the router name")
	cmd.Flags().StringVar(&options.fingerprint, "fingerprint", "", "Sets the router fingerprint")
	cmd.Flags().Uint16Var(&options.cost, "cost", 0, "Specifies the router cost. Default 0.")
	cmd.Flags().BoolVar(&options.noTraversal, "no-traversal", false, "Disallow traversal for this edge router. Default to allowed(false).")
	cmd.Flags().BoolVar(&options.disabled, "disabled", false, "Disabled routers can't connect to controllers")
	cmd.Flags().StringSliceVar(&options.ctrlChanListeners, "ctrl-chan-listener", nil, "Control channel listener address and optional groups (e.g. 'tls:1.2.3.4:6262=group1,group2')")
	cmd.Flags().StringToStringVar(&options.tags, "tags", nil, "Custom management tags")

	options.AddCommonFlags(cmd)

	return cmd
}

// runUpdateRouter update a new router on the Ziti Edge Controller
func runUpdateRouter(o *updateRouterOptions) error {
	id, err := api.MapNameToID(util.FabricAPI, "routers", &o.Options, o.Args[0])
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		api.SetJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("fingerprint") {
		api.SetJSONValue(entityData, o.fingerprint, "fingerprint")
		change = true
	}

	if o.Cmd.Flags().Changed("cost") {
		api.SetJSONValue(entityData, o.cost, "cost")
		change = true
	}

	if o.Cmd.Flags().Changed("no-traversal") {
		api.SetJSONValue(entityData, o.noTraversal, "noTraversal")
		change = true
	}

	if o.Cmd.Flags().Changed("disabled") {
		api.SetJSONValue(entityData, o.disabled, "disabled")
		change = true
	}

	if o.Cmd.Flags().Changed("ctrl-chan-listener") {
		api.SetJSONValue(entityData, api.ParseCtrlChanListeners(o.ctrlChanListeners), "ctrlChanListeners")
		change = true
	}

	if o.Cmd.Flags().Changed("tags") {
		api.SetJSONValue(entityData, o.tags, "tags")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	_, err = patchEntityOfType(fmt.Sprintf("routers/%v", id), entityData.String(), &o.Options)
	return err
}
