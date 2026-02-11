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
	"io"

	"github.com/openziti/ziti/v2/ziti/cmd/api"

	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type updateEdgeRouterOptions struct {
	api.EntityOptions
	name              string
	isTunnelerEnabled bool
	roleAttributes    []string
	appData           map[string]string
	usePut            bool
	cost              uint16
	noTraversal       bool
	disabled          bool
	ctrlChanListeners []string
}

func newUpdateEdgeRouterCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updateEdgeRouterOptions{
		EntityOptions: api.NewEntityOptions(out, errOut),
	}

	cmd := &cobra.Command{
		Use:     "edge-router <idOrName>",
		Aliases: []string{"er"},
		Short:   "updates an edge router managed by the Ziti Edge Controller",
		Long:    "updates an edge router managed by the Ziti Edge Controller",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Cmd = cmd
			options.Args = args
			return runUpdateEdgeRouter(options)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name of the edge router")
	cmd.Flags().BoolVarP(&options.isTunnelerEnabled, "tunneler-enabled", "t", false, "Can this edge router be used as a tunneler")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil,
		"comma-separated role attributes for the router. Use '' to unset.")
	cmd.Flags().StringToStringVar(&options.appData, "app-data", nil, "Custom application data")
	cmd.Flags().BoolVar(&options.usePut, "use-put", false, "Use PUT to when making the request")
	cmd.Flags().Uint16Var(&options.cost, "cost", 0, "Specifies the router cost. Default 0.")
	cmd.Flags().BoolVar(&options.noTraversal, "no-traversal", false, "Disallow traversal for this edge router. Default to allowed(false).")
	cmd.Flags().BoolVar(&options.disabled, "disabled", false, "Disabled routers can't connect to controllers")
	cmd.Flags().StringSliceVar(&options.ctrlChanListeners, "ctrl-chan-listener", nil, "Control channel listener address(es) for the router")

	options.AddCommonFlags(cmd)

	return cmd
}

// runUpdateEdgeRouter update a new edgeRouter on the Ziti Edge Controller
func runUpdateEdgeRouter(o *updateEdgeRouterOptions) error {
	id, err := mapNameToID("edge-routers", o.Args[0], o.Options)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		api.SetJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("tunneler-enabled") {
		api.SetJSONValue(entityData, o.isTunnelerEnabled, "isTunnelerEnabled")
		change = true
	}

	if o.Cmd.Flags().Changed("role-attributes") {
		api.SetJSONValue(entityData, o.roleAttributes, "roleAttributes")
		change = true
	}

	if o.TagsProvided() {
		o.SetTags(entityData)
		change = true
	}

	if o.Cmd.Flags().Changed("app-data") {
		api.SetJSONValue(entityData, o.appData, "appData")
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
		api.SetJSONValue(entityData, o.ctrlChanListeners, "ctrlChanListeners")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	if o.usePut {
		_, err = putEntityOfType(fmt.Sprintf("edge-routers/%v", id), entityData.String(), &o.Options)
	} else {
		_, err = patchEntityOfType(fmt.Sprintf("edge-routers/%v", id), entityData.String(), &o.Options)
	}
	return err
}
