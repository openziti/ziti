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

package tunnel

import (
	"github.com/openziti/edge/tunnel/intercept/host"
	"github.com/spf13/cobra"
)

func NewHostCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "host",
		Short:   "Run in 'host' mode",
		Long:    "The 'host' mode will only host services",
		Args:    cobra.ExactArgs(0),
		RunE:    runHost,
		PostRun: rootPostRun,
	}
}

func runHost(cmd *cobra.Command, args []string) error {
	root := cmd.Root()
	if !root.Flag(resolverCfgFlag).Changed {
		_ = root.PersistentFlags().Set(resolverCfgFlag, "")
	}
	interceptor = host.New()
	return nil
}
