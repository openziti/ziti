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

package enroll

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
)

func NewEnrollCmd(p common.OptionsProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enroll",
		Short: "Enroll Routers and Identities",
	}

	enrollRouter := NewEnrollEdgeRouterCmd()
	enrollRouter.Use = "edge-router"

	enrollIdentity := NewEnrollIdentityCommand(p)
	enrollIdentity.Use = "identity path/to/jwt"

	cmd.AddCommand(enrollRouter)
	cmd.AddCommand(enrollIdentity)

	return cmd
}
