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
	"io"

	"github.com/Jeffail/gabs"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/factory"
	"github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/util"
	"github.com/spf13/cobra"
)

// NewCmdEdge creates a command object for the "controller" command
func NewCmdEdge(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := util.NewEmptyParentCmd("edge", "Interact with Ziti Edge Components")
	cmd.AddCommand(newCmdEdgeController(f, out, errOut))
	return cmd
}

// commonOptions are common options for edge controller commands
type commonOptions struct {
	common.CommonOptions
	OutputJSONResponse bool
}

// newCmdEdgeController creates a command object for the "edge controller" command
func newCmdEdgeController(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := util.NewEmptyParentCmd("controller", "Interact with a Ziti Edge Controller")
	cmd.AddCommand(newCreateCmd(f, out, errOut))
	cmd.AddCommand(newDeleteCmd(f, out, errOut))
	cmd.AddCommand(newLoginCmd(f, out, errOut))
	cmd.AddCommand(newListCmd(f, out, errOut))
	cmd.AddCommand(newUpdateCmd(f, out, errOut))
	cmd.AddCommand(newVersionCmd(f, out, errOut))
	cmd.AddCommand(newPolicyAdivsorCmd(f, out, errOut))
	cmd.AddCommand(newVerifyCmd(f, out, errOut))
	return cmd
}

func setJSONValue(container *gabs.Container, value interface{}, path ...string) {
	if _, err := container.Set(value, path...); err != nil {
		panic(err)
	}
}
