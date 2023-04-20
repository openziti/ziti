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
	"github.com/fatih/color"
	"github.com/openziti/ziti/ziti/cmd/api"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
)

type reEnrollEdgeRouterOptions struct {
	api.EntityOptions
	jwtOutputFile string
}

func newReEnrollEdgeRouterCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &reEnrollEdgeRouterOptions{
		EntityOptions: api.NewEntityOptions(out, errOut),
	}

	cmd := &cobra.Command{
		Use:     "edge-router <idOrName>",
		Aliases: []string{"er"},
		Short:   "re-enrolls an edge router managed by the Ziti Edge Controller",
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runReEnrollEdgeRouter(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.jwtOutputFile, "jwt-output-file", "o", "", "File to which to output the JWT used for enrolling the edge router")

	options.Options.AddCommonFlags(cmd)

	return cmd
}

// runReEnrollEdgeRouter implements the command to create a gateway on the edge controller
func runReEnrollEdgeRouter(o *reEnrollEdgeRouterOptions) error {
	id, err := mapNameToID("edge-routers", o.Args[0], o.Options)
	if err != nil {
		return err
	}

	_, err = postEntityOfType(fmt.Sprintf("edge-routers/%v/re-enroll", id), "", &o.Options)
	if err != nil {
		o.Printf("re-enroll edge-router with %v: %v\n", id, color.New(color.FgRed, color.Bold).Sprint("FAIL"))
		return err
	}
	o.Printf("re-enroll edge-router with id %v: %v\n", id, color.New(color.FgGreen, color.Bold).Sprint("OK"))

	if o.jwtOutputFile != "" {
		if err = getEdgeRouterJwt(o.EntityOptions.Options, o.jwtOutputFile, id); err != nil {
			return err
		}
	}
	return nil
}
