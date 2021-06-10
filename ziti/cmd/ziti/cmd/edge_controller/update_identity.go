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
	"fmt"
	"github.com/openziti/sdk-golang/ziti"
	"io"
	"math"
	"strings"

	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

type updateIdentityOptions struct {
	edgeOptions
	name                     string
	roleAttributes           []string
	defaultHostingPrecedence string
	defaultHostingCost       uint16
	serviceCosts             map[string]int
	servicePrecedences       map[string]string
	tags                     map[string]string
	appData                  map[string]string
}

func newUpdateIdentityCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updateIdentityOptions{
		edgeOptions: edgeOptions{
			CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "identity <idOrName>",
		Short: "updates a identity managed by the Ziti Edge Controller",
		Long:  "updates a identity managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdateIdentity(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	options.AddCommonFlags(cmd)
	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name of the identity")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil,
		"Set role attributes of the identity. Use --role-attributes '' to set an empty list")
	cmd.Flags().StringVarP(&options.defaultHostingPrecedence, "default-hosting-precedence", "p", "", "Default precedence to use when hosting services using this identity [default,required,failed]")
	cmd.Flags().Uint16VarP(&options.defaultHostingCost, "default-hosting-cost", "c", 0, "Default cost to use when hosting services using this identity")
	cmd.Flags().StringToIntVar(&options.serviceCosts, "service-costs", map[string]int{}, "Per-service hosting costs")
	cmd.Flags().StringToStringVar(&options.servicePrecedences, "service-precedences", map[string]string{}, "Per-service hosting precedences")
	cmd.Flags().StringToStringVar(&options.tags, "tags", nil, "Custom management tags")
	cmd.Flags().StringToStringVar(&options.appData, "app-data", nil, "Custom application data")

	return cmd
}

// runUpdateIdentity update a new identity on the Ziti Edge Controller
func runUpdateIdentity(o *updateIdentityOptions) error {
	id, err := mapNameToID("identities", o.Args[0], o.edgeOptions)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		setJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("role-attributes") {
		setJSONValue(entityData, o.roleAttributes, "roleAttributes")
		change = true
	}

	if o.Cmd.Flags().Changed("default-hosting-cost") {
		setJSONValue(entityData, o.defaultHostingCost, "defaultHostingCost")
		change = true
	}

	if o.Cmd.Flags().Changed("default-hosting-precedence") {
		prec, err := normalizeAndValidatePrecedence(o.defaultHostingPrecedence)
		if err != nil {
			return err
		}
		setJSONValue(entityData, prec, "defaultHostingPrecedence")
		change = true
	}

	if o.Cmd.Flags().Changed("service-costs") {
		for k, v := range o.serviceCosts {
			if v < 0 || v > math.MaxUint16 {
				return errors.Errorf("hosting costs must be in the range %v-%v", 0, math.MaxUint16)
			}
			id, err := mapNameToID("services", k, o.edgeOptions)
			if err != nil {
				return err
			}
			delete(o.serviceCosts, k)
			o.serviceCosts[id] = v
		}
		setJSONValue(entityData, o.serviceCosts, "serviceHostingCosts")
		change = true
	}

	if o.Cmd.Flags().Changed("service-precedences") {
		for k, v := range o.servicePrecedences {
			id, err := mapNameToID("services", k, o.edgeOptions)
			if err != nil {
				return err
			}

			prec, err := normalizeAndValidatePrecedence(v)
			if err != nil {
				return err
			}

			delete(o.servicePrecedences, k)
			o.servicePrecedences[id] = prec
		}
		setJSONValue(entityData, o.servicePrecedences, "serviceHostingPrecedences")
		change = true
	}

	if o.Cmd.Flags().Changed("tags") {
		setJSONValue(entityData, o.tags, "tags")
		change = true
	}

	if o.Cmd.Flags().Changed("app-data") {
		setJSONValue(entityData, o.appData, "appData")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	_, err = patchEntityOfType(fmt.Sprintf("identities/%v", id), entityData.String(), &o.edgeOptions)
	return err
}

func normalizeAndValidatePrecedence(val string) (string, error) {
	normalized := strings.ToLower(val)
	prec := ziti.GetPrecedenceForLabel(normalized)
	if prec.String() != normalized {
		return "", errors.Errorf("invalid precedence %v. valid valids [default, required, failed]", val)
	}
	return normalized, nil
}
