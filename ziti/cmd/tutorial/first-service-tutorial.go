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
	_ "embed"
	"github.com/openziti/runzmd"
	"github.com/openziti/runzmd/actionz"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"time"
)

//go:embed tutorials/first-service.md
var firstServiceTutorialSource []byte

//go:embed plain_echo_server.go
var plainEchoServerSource string

//go:embed plain_echo_client.go
var plainEchoClientSource string

//go:embed ziti_echo_client.go
var zitiEchoClientSource string

//go:embed ziti_echo_server.go
var zitiEchoServerSource string

type firstServiceTutorialOptions struct {
	api.Options
	TutorialOptions
}

func newFirstServiceTutorialCmd(p common.OptionsProvider) *cobra.Command {
	options := &firstServiceTutorialOptions{
		Options: api.Options{
			CommonOptions: p(),
		},
	}

	cmd := &cobra.Command{
		Use:   "first-service",
		Short: "Walks you through creating a service, identity and policies",
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
	cmd.Flags().BoolVar(&options.AssumeDefault, "assume-default", false, "Non-interactive mode, assuming default input when user input is required")
	options.AddCommonFlags(cmd)

	return cmd
}

func (self *firstServiceTutorialOptions) run() error {
	t := runzmd.NewRunner()
	t.NewLinePause = self.NewlinePause
	t.AssumeDefault = self.AssumeDefault

	t.RegisterActionHandler("ziti", &actionz.ZitiRunnerAction{})
	t.RegisterActionHandler("ziti-login", &actionz.ZitiLoginAction{
		LoginParams: &self.TutorialOptions,
	})
	t.RegisterActionHandler("keep-session-alive", &actionz.KeepSessionAliveAction{})
	t.RegisterActionHandler("select-edge-router", &actionz.SelectEdgeRouterAction{})

	plainEchoServerActions := &PlainEchoServerActions{}
	t.RegisterActionHandlerF("run-plain-echo-server", plainEchoServerActions.Start)
	t.RegisterActionHandlerF("stop-plain-echo-server", plainEchoServerActions.Stop)

	zitiEchoServerActions := &ZitiEchoServerActions{}
	t.RegisterActionHandlerF("run-ziti-echo-server", zitiEchoServerActions.Start)
	t.RegisterActionHandlerF("stop-ziti-echo-server", zitiEchoServerActions.Stop)

	showActionHandler := runzmd.NewShowActionHandler()
	showActionHandler.Add("plain_echo_server.go", plainEchoServerSource)
	showActionHandler.Add("plain_echo_client.go", plainEchoClientSource)
	showActionHandler.Add("ziti_echo_client.go", zitiEchoClientSource)
	showActionHandler.Add("ziti_echo_server.go", zitiEchoServerSource)
	t.RegisterActionHandler("show", showActionHandler)

	return t.Run(firstServiceTutorialSource)
}

type PlainEchoServerActions struct {
	server plainEchoServer
}

func (self *PlainEchoServerActions) Start(ctx *runzmd.ActionContext) error {
	if !ctx.Runner.AssumeDefault {
		start, err := runzmd.AskYesNoWithDefault("Start plain-echo-server? [Y/N] (default Y): ", true)
		if err != nil {
			return err
		}
		if !start {
			return nil
		}
	}

	if err := self.server.run(); err != nil {
		return err
	}
	ctx.Runner.AddVariable("port", self.server.Port)
	return nil
}

func (self *PlainEchoServerActions) Stop(ctx *runzmd.ActionContext) error {
	if !ctx.Runner.AssumeDefault {
		start, err := runzmd.AskYesNoWithDefault("Stop plain-echo-server? [Y/N] (default Y): ", true)
		if err != nil {
			return err
		}
		if !start {
			return nil
		}
	}

	if self.server.listener != nil {
		return self.server.stop()
	}
	return nil
}

type ZitiEchoServerActions struct {
	server zitiEchoServer
}

func (self *ZitiEchoServerActions) Start(ctx *runzmd.ActionContext) error {
	logrus.SetLevel(logrus.WarnLevel)

	self.server.identityJson = "echo-server.json"
	if !ctx.Runner.AssumeDefault {
		start, err := runzmd.AskYesNoWithDefault("Start ziti-echo-server? [Y/N] (default Y): ", true)
		if err != nil {
			return err
		}
		if !start {
			return nil
		}
	}

	if err := self.server.run(); err != nil {
		return err
	}
	return nil
}

func (self *ZitiEchoServerActions) Stop(*runzmd.ActionContext) error {
	if self.server.listener != nil {
		return self.server.stop()
	}
	return nil
}
