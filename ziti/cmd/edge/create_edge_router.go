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
	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/api"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
	"os"
)

type createEdgeRouterOptions struct {
	api.EntityOptions
	isTunnelerEnabled bool
	roleAttributes    []string
	jwtOutputFile     string
	appData           map[string]string
	cost              uint16
	noTraversal       bool
	disabled          bool
}

func NewCreateEdgeRouterCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createEdgeRouterOptions{
		EntityOptions: api.NewEntityOptions(out, errOut),
	}

	cmd := &cobra.Command{
		Use:     "edge-router <name>",
		Aliases: []string{"gateway", "er"},
		Short:   "creates an edge router managed by the Ziti Edge Controller",
		Long:    "creates an edge router managed by the Ziti Edge Controller",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateEdgeRouter(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil, "comma-separated role attributes for the new router")
	cmd.Flags().BoolVarP(&options.isTunnelerEnabled, "tunneler-enabled", "t", false, "Can this edge router be used as a tunneler")
	cmd.Flags().StringVarP(&options.jwtOutputFile, "jwt-output-file", "o", "", "File to which to output the JWT used for enrolling the edge router")
	cmd.Flags().StringToStringVar(&options.appData, "app-data", nil, "Custom application data")
	cmd.Flags().Uint16Var(&options.cost, "cost", 0, "Specifies the router cost. Default 0.")
	cmd.Flags().BoolVar(&options.noTraversal, "no-traversal", false, "Disallow traversal for this edge router. Default to allowed(false).")
	cmd.Flags().BoolVar(&options.disabled, "disabled", false, "Disabled routers can't connect to controllers")

	options.AddCommonFlags(cmd)

	return cmd
}

// runCreateEdgeRouter implements the command to create a gateway on the edge controller
func runCreateEdgeRouter(o *createEdgeRouterOptions) error {
	entityData := gabs.New()
	api.SetJSONValue(entityData, o.Args[0], "name")
	api.SetJSONValue(entityData, o.isTunnelerEnabled, "isTunnelerEnabled")
	api.SetJSONValue(entityData, o.roleAttributes, "roleAttributes")
	api.SetJSONValue(entityData, o.appData, "appData")
	api.SetJSONValue(entityData, o.cost, "cost")
	api.SetJSONValue(entityData, o.noTraversal, "noTraversal")
	api.SetJSONValue(entityData, o.disabled, "disabled")
	o.SetTags(entityData)

	result, err := CreateEntityOfType("edge-routers", entityData.String(), &o.Options)
	if err := o.LogCreateResult("edge router", result, err); err != nil {
		return err
	}

	if o.jwtOutputFile != "" {
		id := result.S("data", "id").Data().(string)
		if err := getEdgeRouterJwt(o.EntityOptions.Options, o.jwtOutputFile, id); err != nil {
			return err
		}
	}
	return nil
}

func getEdgeRouterJwt(o api.Options, jwtOutputFile string, id string) error {
	newRouter, err := DetailEntityOfType("edge-routers", id, o.OutputJSONResponse, o.Out, o.Timeout, o.Verbose)
	if err != nil {
		return err
	}

	if newRouter == nil {
		return fmt.Errorf("no error during edge router creation, but edge router with id %v not found... unable to extract JWT", id)
	}

	jwt := newRouter.Path("enrollmentJwt").Data().(string)
	if jwt == "" {
		return fmt.Errorf("enrollment JWT not present for new edge router")
	}

	if err := os.WriteFile(jwtOutputFile, []byte(jwt), 0600); err != nil {
		fmt.Printf("Failed to write JWT to file(%v)\n", jwtOutputFile)
		return err
	}

	jwtExpiration := newRouter.Path("enrollmentExpiresAt").Data().(string)
	if jwtExpiration != "" {
		fmt.Printf("Enrollment expires at %v\n", jwtExpiration)
	}

	return err
}
