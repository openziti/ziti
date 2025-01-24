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
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/enroll"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"io"
)

var ExtraEdgeCommands []func(p common.OptionsProvider) *cobra.Command

// NewCmdEdge creates a command object for the "controller" command
func NewCmdEdge(out io.Writer, errOut io.Writer, p common.OptionsProvider) *cobra.Command {
	cmd := util.NewEmptyParentCmd("edge", "Manage the Edge components of a Ziti network using the Ziti Edge REST API")

	cmd.AddCommand(newCreateCmd(out, errOut))
	cmd.AddCommand(newDeleteCmd(out, errOut))
	cmd.AddCommand(NewLoginCmd(out, errOut))
	cmd.AddCommand(newLogoutCmd(out, errOut))
	cmd.AddCommand(newUseCmd(out, errOut))
	cmd.AddCommand(newListCmd(out, errOut))
	cmd.AddCommand(newUpdateCmd(out, errOut))
	cmd.AddCommand(newVersionCmd(out, errOut))
	cmd.AddCommand(newPolicyAdivsorCmd(out, errOut))
	cmd.AddCommand(newVerifyCmd(out, errOut))
	cmd.AddCommand(newDbCmd(out, errOut))
	cmd.AddCommand(newTraceCmd(out, errOut))
	cmd.AddCommand(newTraceRouteCmd(out, errOut))
	cmd.AddCommand(newShowCmd(out, errOut))
	cmd.AddCommand(newReEnrollCmd(out, errOut))
	cmd.AddCommand(newValidateCommand(p))
	cmd.AddCommand(enroll.NewEnrollIdentityCommand(p))

	for _, cmdF := range ExtraEdgeCommands {
		cmd.AddCommand(cmdF(p))
	}

	return cmd
}

func newValidateCommand(p common.OptionsProvider) *cobra.Command {
	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "validate model data",
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	validateCmd.AddCommand(NewValidateServiceHostingCmd(p))
	return validateCmd
}
