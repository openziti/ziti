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

package common

import (
	"fmt"
	"github.com/openziti/foundation/v2/tlz"
	"github.com/openziti/ziti/common/version"
	"github.com/spf13/cobra"
)

func NewVersionCmd() *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show component version",
		Run: func(cmd *cobra.Command, args []string) {
			if verbose {
				fmt.Printf("Version:      %s\n", version.GetVersion())
				fmt.Printf("Revision:     %s\n", version.GetRevision())
				fmt.Printf("Build Date:   %s\n", version.GetBuildDate())
				fmt.Printf("Go Version:   %s\n", version.GetGoVersion())
				fmt.Printf("OS/Arch:      %s/%s\n", version.GetOS(), version.GetArchitecture())
				fmt.Printf("FIPS mode:    %v\n", tlz.FipsEnabled())
			} else {
				fmt.Println(version.GetVersion())
			}
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show verbose version information")
	return cmd
}
