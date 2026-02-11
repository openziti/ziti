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
	"io"

	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/openziti/ziti/v2/ziti/enroll"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/spf13/cobra"
)

var ExtraEdgeCommands []func(p common.OptionsProvider) *cobra.Command

// NewCmdEdge creates a command object for the "controller" command
func NewCmdEdge(out io.Writer, errOut io.Writer, p common.OptionsProvider) *cobra.Command {
	return newCmdEdgeInternal(out, errOut, p, true)
}

// NewCmdEdgeV2 creates a command object for the "controller" command without CRUD subcommands
// (they are consolidated at the top level in V2)
func NewCmdEdgeV2(out io.Writer, errOut io.Writer, p common.OptionsProvider) *cobra.Command {
	return newCmdEdgeInternal(out, errOut, p, false)
}

func newCmdEdgeInternal(out io.Writer, errOut io.Writer, p common.OptionsProvider, v1Layout bool) *cobra.Command {
	cmd := util.NewEmptyParentCmd("edge", "Manage the Edge components of a Ziti network using the Ziti Edge REST API")

	if v1Layout {
		cmd.AddCommand(newCreateCmd(out, errOut))
		cmd.AddCommand(newDeleteCmd(out, errOut))
		cmd.AddCommand(newListCmd(out, errOut))
		cmd.AddCommand(newUpdateCmd(out, errOut))
		cmd.AddCommand(NewLoginCmd(out, errOut))
		cmd.AddCommand(NewLogoutCmd(out, errOut))
		cmd.AddCommand(NewUseCmd(out, errOut))
		cmd.AddCommand(newValidateCommand(p))
		cmd.AddCommand(newDbCmd(out, errOut))
		cmd.AddCommand(NewPolicyAdvisorCmd(out, errOut))
		cmd.AddCommand(newVerifyCmd(out, errOut))
		cmd.AddCommand(NewTraceRouteCmd(out, errOut))
		cmd.AddCommand(newReEnrollCmd(out, errOut))
		cmd.AddCommand(enroll.NewEnrollIdentityCommand(p))
		cmd.AddCommand(NewTraceCmd(out, errOut))
		cmd.AddCommand(NewVersionCmd(out, errOut))
		cmd.AddCommand(NewShowCmd(out, errOut))
	}

	for _, cmdF := range ExtraEdgeCommands {
		cmd.AddCommand(cmdF(p))
	}

	return cmd
}

func newValidateCommand(p common.OptionsProvider) *cobra.Command {
	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "validate model data",
	}

	validateCmd.AddCommand(NewValidateServiceHostingCmd(p))
	return validateCmd
}
