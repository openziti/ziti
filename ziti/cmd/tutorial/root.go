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

package tutorial

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/edge"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"time"
)

func init() {
	edge.ExtraEdgeCommands = append(edge.ExtraEdgeCommands, NewTutorialCmd)
}

func NewTutorialCmd(p common.OptionsProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tutorial",
		Short: "Interactive tutorials for learning about Ziti",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newFirstServiceTutorialCmd(p))
	cmd.AddCommand(newPlainEchoServerCmd(p))
	cmd.AddCommand(newPlainEchoClientCmd(p))
	cmd.AddCommand(newZitiEchoClientCmd(p))
	cmd.AddCommand(newZitiEchoServerCmd(p))

	return cmd
}

type TutorialOptions struct {
	ControllerUrl string
	Username      string
	Password      string
	NewlinePause  time.Duration
	AssumeDefault bool
}

func (self *TutorialOptions) GetControllerUrl() string {
	return self.ControllerUrl
}

func (self *TutorialOptions) GetUsername() string {
	return self.Username
}

func (self *TutorialOptions) GetPassword() string {
	return self.Password
}
