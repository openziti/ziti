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

package demo

import (
	_ "embed"
	"github.com/openziti/runzmd"
	"github.com/openziti/runzmd/actionz"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/tutorial"
	"github.com/spf13/cobra"
	"time"
)

//go:embed setup-scripts/multi-sdk-hosted.md
var multiSdkHostedScriptSource []byte

type multiSdkHosted struct {
	api.Options
	tutorial.TutorialOptions
	interactive bool
}

func newMultiSdkHostedCmd(p common.OptionsProvider) *cobra.Command {
	options := &multiSdkHosted{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "multi-sdk-hosted",
		Short: "Walks you through hosting configuration for an echo service hosted by a multiple sdk applications",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.run()
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVar(&options.ControllerUrl, "controller-url", "", "The Ziti controller URL to use")
	cmd.Flags().StringVarP(&options.Username, "username", "u", "", "The Ziti controller username to use")
	cmd.Flags().StringVarP(&options.Password, "password", "p", "", "The Ziti controller password to use")
	cmd.Flags().DurationVar(&options.NewlinePause, "newline-pause", time.Millisecond*10, "How long to pause between lines when scrolling")
	cmd.Flags().BoolVar(&options.interactive, "interactive", false, "Interactive mode, waiting for user input")
	options.AddCommonFlags(cmd)

	return cmd
}

func (self *multiSdkHosted) run() error {
	t := runzmd.NewRunner()
	t.NewLinePause = self.NewlinePause
	t.AssumeDefault = !self.interactive

	t.RegisterActionHandler("ziti", &actionz.ZitiRunnerAction{})
	t.RegisterActionHandler("ziti-login", &actionz.ZitiEnsureLoggedIn{
		LoginParams: &self.TutorialOptions,
	})
	t.RegisterActionHandler("keep-session-alive", &actionz.KeepSessionAliveAction{})

	return t.Run(multiSdkHostedScriptSource)
}
