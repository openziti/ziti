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
	"github.com/openziti/ziti/common/enrollment"
	"io"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
)

// NewCmdEdge creates a command object for the "controller" command
func NewCmdEdge(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := util.NewEmptyParentCmd("edge", "Interact with Ziti Edge Components")
	cmd.AddCommand(newCmdEdgeController(f, out, errOut))
	populateEdgeCommands(f, out, errOut, cmd)
	return cmd
}

// edgeOptions are common options for edge controller commands
type edgeOptions struct {
	common.CommonOptions
	OutputJSONRequest  bool
	OutputJSONResponse bool
}

func (options *edgeOptions) OutputRequestJson() bool {
	return options.OutputJSONRequest
}

func (options *edgeOptions) OutputResponseJson() bool {
	return options.OutputJSONResponse
}

func (options *edgeOptions) OutputWriter() io.Writer {
	return options.CommonOptions.Out
}

func (options *edgeOptions) ErrOutputWriter() io.Writer {
	return options.CommonOptions.Err
}
func (options *edgeOptions) AddCommonFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	cmd.Flags().BoolVar(&options.OutputJSONRequest, "output-request-json", false, "Output the full JSON request to the Ziti Edge Controller")
	cmd.Flags().IntVarP(&options.Timeout, "timeout", "", 5, "Timeout for REST operations (specified in seconds)")
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
}

// newCmdEdgeController creates a command object for the "edge controller" command
func newCmdEdgeController(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := util.NewEmptyParentCmd("controller", "Interact with a Ziti Edge Controller")
	populateEdgeCommands(f, out, errOut, cmd)
	return cmd
}

func populateEdgeCommands(f cmdutil.Factory, out io.Writer, errOut io.Writer, cmd *cobra.Command) *cobra.Command {

	cmd.AddCommand(newCreateCmd(f, out, errOut))
	cmd.AddCommand(newDeleteCmd(f, out, errOut))
	cmd.AddCommand(newLoginCmd(f, out, errOut))
	cmd.AddCommand(newListCmd(f, out, errOut))
	cmd.AddCommand(newUpdateCmd(f, out, errOut))
	cmd.AddCommand(newVersionCmd(f, out, errOut))
	cmd.AddCommand(newPolicyAdivsorCmd(f, out, errOut))
	cmd.AddCommand(newVerifyCmd(f, out, errOut))
	cmd.AddCommand(newDbCmd(f, out, errOut))
	cmd.AddCommand(enrollment.NewEnrollCommand())
	return cmd
}

func setJSONValue(container *gabs.Container, value interface{}, path ...string) {
	if _, err := container.Set(value, path...); err != nil {
		panic(err)
	}
}
