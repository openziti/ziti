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
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"io"
	"os"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type createTransitRouterOptions struct {
	api.Options
	jwtOutputFile string
	tags          map[string]string
	cost          uint16
	noTraversal   bool
}

func newCreateTransitRouterCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createTransitRouterOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "transit-router <name>",
		Short: "creates a transit router managed by the Ziti Edge Controller",
		Long:  "creates a transit router managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateTransitRouter(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.jwtOutputFile, "jwt-output-file", "o", "", "File to which to output the JWT used for enrolling the edge router")
	cmd.Flags().StringToStringVar(&options.tags, "tags", nil, "Custom management tags")
	cmd.Flags().Uint16Var(&options.cost, "cost", 0, "Specifies the router cost. Default 0.")
	cmd.Flags().BoolVar(&options.noTraversal, "no-traversal", false, "Disallow traversal for this edge router. Default to allowed(false).")

	options.AddCommonFlags(cmd)

	return cmd
}

// runCreateTransitRouter implements the command to create a gateway on the edge controller
func runCreateTransitRouter(o *createTransitRouterOptions) error {
	entityData := gabs.New()
	api.SetJSONValue(entityData, o.Args[0], "name")
	api.SetJSONValue(entityData, o.tags, "tags")
	api.SetJSONValue(entityData, o.cost, "cost")
	api.SetJSONValue(entityData, o.noTraversal, "noTraversal")

	result, err := CreateEntityOfType("transit-routers", entityData.String(), &o.Options)
	if err := o.LogCreateResult("transit router", result, err); err != nil {
		return err
	}

	if o.jwtOutputFile != "" {
		id := result.S("data", "id").Data().(string)
		if err := getTransitRouterJwt(o, id); err != nil {
			return err
		}
	}
	return nil
}

func getTransitRouterJwt(o *createTransitRouterOptions, id string) error {
	newRouter, err := DetailEntityOfType("transit-routers", id, o.OutputJSONResponse, o.Out, o.Options.Timeout, o.Options.Verbose)
	if err != nil {
		return err
	}

	if newRouter == nil {
		return fmt.Errorf("no error during transit router creation, but edge router with id %v not found... unable to extract JWT", id)
	}

	jwt := newRouter.Path("enrollmentJwt").Data().(string)
	if jwt == "" {
		return fmt.Errorf("enrollment JWT not present for new transit router")
	}

	if err := os.WriteFile(o.jwtOutputFile, []byte(jwt), 0600); err != nil {
		fmt.Printf("Failed to write JWT to file(%v)\n", o.jwtOutputFile)
		return err
	}

	jwtExpiration := newRouter.Path("enrollmentExpiresAt").Data().(string)
	if jwtExpiration != "" {
		fmt.Printf("Enrollment expires at %v\n", jwtExpiration)
	}

	return err
}
