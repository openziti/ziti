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
	"crypto/sha1"
	"fmt"

	"github.com/Jeffail/gabs"
	"github.com/openziti/identity/certtools"
	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/spf13/cobra"
)

type createRouterOptions struct {
	api.Options
	name                       string
	cost                       uint16
	tags                       map[string]string
	noTraversal                bool
	disabled                   bool
	bootstrapCtrlChanListeners []string
	configs                    []string
}

// deprecationNotice is printed on every use. This command creates a router outside enrollment, taking its
// id from the certificate's common name, which dates from when a router had one identifier rather than a
// separate id and name. It is the only way to give a router a caller-chosen id, and that is what makes a
// deleted router id able to come back.
const deprecationNotice = `WARNING: 'ziti fabric create router' is deprecated and will be removed in a
future release. It creates a router outside the enrollment process, with its id taken from the
certificate. Enroll routers instead, which assigns the id. If you rely on this command, please open an
issue describing your use case.`

// newCreateRouterCmd creates the 'fabric create router' command for the given entity type
//
// Deprecated: creating a router outside enrollment is deprecated. See deprecationNotice.
func newCreateRouterCmd(p common.OptionsProvider) *cobra.Command {
	options := &createRouterOptions{
		Options: api.Options{
			CommonOptions: p(),
		},
		tags: make(map[string]string),
	}

	cmd := &cobra.Command{
		Use:        "router <path-to-cert>",
		Short:      "creates a router managed by the Ziti Controller (deprecated)",
		Long:       deprecationNotice,
		Deprecated: "enroll routers instead; this creates a router outside enrollment with an id taken from the certificate",
		Args:       cobra.MinimumNArgs(1),
		RunE:       options.createRouter,
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringToStringVarP(&options.tags, "tags", "t", nil, "Add tags to router definition")
	cmd.Flags().StringVar(&options.name, "name", "", "Specifies the router name. If not specified, the id in the controller cert will be used")
	cmd.Flags().Uint16Var(&options.cost, "cost", 0, "Specifies the router cost. Default 0.")
	cmd.Flags().BoolVar(&options.noTraversal, "no-traversal", false, "Disallow traversal for this edge router. Default to allowed(false).")
	cmd.Flags().BoolVar(&options.disabled, "disabled", false, "Disabled routers can't connect to controllers")
	cmd.Flags().StringSliceVar(&options.bootstrapCtrlChanListeners, "bootstrap-ctrl-chan-listener", nil, "Bootstrap control channel listener address and optional groups. The router will update this automatically once connected. (e.g. 'tls:1.2.3.4:6262=group1,group2')")
	cmd.Flags().StringSliceVar(&options.configs, "config", nil, "Config IDs to associate with this router")

	options.AddCommonFlags(cmd)

	return cmd
}

// createRouter implements the command to create a router
func (o *createRouterOptions) createRouter(_ *cobra.Command, args []string) error {
	cert, err := certtools.LoadCertFromFile(args[0])
	if err != nil {
		return err
	}

	entityData := gabs.New()
	id := cert[0].Subject.CommonName
	name := id
	if o.name != "" {
		name = o.name
	}
	fingerprint := fmt.Sprintf("%x", sha1.Sum(cert[0].Raw))
	api.SetJSONValue(entityData, id, "id")
	api.SetJSONValue(entityData, name, "name")
	api.SetJSONValue(entityData, fingerprint, "fingerprint")
	api.SetJSONValue(entityData, o.tags, "tags")
	api.SetJSONValue(entityData, o.cost, "cost")
	api.SetJSONValue(entityData, o.noTraversal, "noTraversal")
	api.SetJSONValue(entityData, o.disabled, "disabled")
	if len(o.bootstrapCtrlChanListeners) > 0 {
		api.SetJSONValue(entityData, api.ParseCtrlChanListeners(o.bootstrapCtrlChanListeners), "ctrlChanListeners")
	}
	if len(o.configs) > 0 {
		api.SetJSONValue(entityData, o.configs, "configs")
	}
	result, err := createEntityOfType("routers", entityData.String(), &o.Options)
	if err != nil {
		return err
	}

	if !o.OutputJSONResponse {
		id := result.S("data", "id").Data()
		_, err = fmt.Fprintf(o.Out, "New %v %v created with id: %v\n", "router", name, id)
		return err
	}
	return nil
}
