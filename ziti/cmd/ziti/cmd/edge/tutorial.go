package edge

import (
	_ "embed"
	"fmt"
	"github.com/fatih/color"
	"github.com/openziti/foundation/util/term"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/tutorial"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed tutorials/first-service.md
var firstServiceTutorialSource []byte

//go:embed tutorial_plain_echo_server.go
var plainEchoServerSource string

//go:embed tutorial_plain_echo_client.go
var plainEchoClientSource string

//go:embed tutorial_ziti_echo_client.go
var zitiEchoClientSource string

//go:embed tutorial_ziti_echo_server.go
var zitiEchoServerSource string

func newTutorialCmd(p common.OptionsProvider) *cobra.Command {
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

type tutorialOptions struct {
	controllerUrl string
	username      string
	password      string
	newlinePause  time.Duration
	assumeDefault bool
}

type firstServiceTutorialOptions struct {
	edgeOptions
	tutorialOptions
}

func newFirstServiceTutorialCmd(p common.OptionsProvider) *cobra.Command {
	options := &firstServiceTutorialOptions{
		edgeOptions: edgeOptions{
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
	cmd.Flags().StringVar(&options.controllerUrl, "controller-url", "", "The Ziti controller URL to use")
	cmd.Flags().StringVarP(&options.username, "username", "u", "", "The Ziti controller username to use")
	cmd.Flags().StringVarP(&options.password, "password", "p", "", "The Ziti controller password to use")
	cmd.Flags().DurationVar(&options.newlinePause, "newline-pause", time.Millisecond*10, "How long to pause between lines when scrolling")
	cmd.Flags().BoolVar(&options.assumeDefault, "assume-default", false, "Non-interactive mode, assuming default input when user input is required")
	options.AddCommonFlags(cmd)

	return cmd
}

func (self *firstServiceTutorialOptions) run() error {
	t := tutorial.NewRunner()
	t.NewLinePause = self.newlinePause
	t.AssumeDefault = self.assumeDefault

	t.RegisterActionHandler("ziti", &ZitiRunnerAction{})
	t.RegisterActionHandler("ziti-login", &ZitiLoginAction{
		options: &self.tutorialOptions,
	})
	t.RegisterActionHandler("keep-session-alive", &KeepSessionAliveAction{})
	t.RegisterActionHandler("select-edge-router", &SelectEdgeRouterAction{})

	plainEchoServerActions := &PlainEchoServerActions{}
	t.RegisterActionHandlerF("run-plain-echo-server", plainEchoServerActions.Start)
	t.RegisterActionHandlerF("stop-plain-echo-server", plainEchoServerActions.Stop)

	zitiEchoServerActions := &ZitiEchoServerActions{}
	t.RegisterActionHandlerF("run-ziti-echo-server", zitiEchoServerActions.Start)
	t.RegisterActionHandlerF("stop-ziti-echo-server", zitiEchoServerActions.Stop)

	showActionHandler := tutorial.NewShowActionHandler()
	showActionHandler.Add("tutorial_plain_echo_server.go", plainEchoServerSource)
	showActionHandler.Add("tutorial_plain_echo_client.go", plainEchoClientSource)
	showActionHandler.Add("tutorial_ziti_echo_client.go", zitiEchoClientSource)
	showActionHandler.Add("tutorial_ziti_echo_server.go", zitiEchoServerSource)
	t.RegisterActionHandler("show", showActionHandler)

	return t.Run(firstServiceTutorialSource)
}

type ZitiLoginAction struct {
	options *tutorialOptions
}

func (self *ZitiLoginAction) Execute(ctx *tutorial.ActionContext) error {
	cmd := "ziti edge login --ignore-config"
	if self.options.controllerUrl != "" {
		cmd += " " + self.options.controllerUrl
	}
	if self.options.username != "" {
		cmd += " --username " + self.options.username
	}
	if self.options.password != "" {
		cmd += " --password " + self.options.password
	}
	ctx.Body = cmd
	return (&ZitiRunnerAction{}).Execute(ctx)
}

type ZitiRunnerAction struct{}

func (self *ZitiRunnerAction) Execute(ctx *tutorial.ActionContext) error {
	if strings.EqualFold("true", ctx.Headers["templatize"]) {
		body, err := ctx.Runner.Template(ctx.Body)
		if err != nil {
			return err
		}
		ctx.Body = body
	}
	lines := strings.Split(ctx.Body, "\n")
	var cmds [][]string
	buf := &strings.Builder{}
	buf.WriteString("About to execute:\n\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 0 {
			params := tutorial.ParseArgumentsWithStrings(line)
			if params[0] != "ziti" {
				return errors.Errorf("invalid parameter for ziti action, must start with 'ziti': %v", ctx.Body)
			}
			params[0] = line
			cmds = append(cmds, params)
			ctx.Runner.LeftPadBuilder(buf)
			buf.WriteString("  ")
			buf.WriteString(color.New(color.Bold).Sprintf(line))
			buf.WriteRune('\n')
		}
	}
	buf.WriteRune('\n')
	ctx.Runner.LeftPadBuilder(buf)
	buf.WriteString("Continue [Y/N] (default Y): ")

	if !ctx.Runner.AssumeDefault {
		tutorial.Continue(buf.String(), true)
	}

	fmt.Println("")
	c := color.New(color.FgBlue, color.Bold)

	colorStdOut := !strings.EqualFold("false", ctx.Headers["colorStdOut"])

	allowRetry := strings.EqualFold("true", ctx.Headers["allowRetry"])
	failOk := strings.EqualFold("true", ctx.Headers["failOk"])
	for _, cmd := range cmds {
		_, _ = c.Printf("$ %v\n", cmd[0])
		done := false
		for !done {
			if err := tutorial.Exec(os.Args[0], colorStdOut, cmd[1:]...); err != nil {
				if failOk {
					return nil
				}
				if allowRetry {
					retry, err2 := tutorial.AskYesNoWithDefault(fmt.Sprintf("operation failed with err: %v. Retry [Y/N] (default Y):", err), true)
					if err2 != nil {
						fmt.Printf("error while asking about retry: %v\n", err2)
						return err
					}
					if !retry {
						return err
					}
				} else {
					return err
				}
			} else {
				done = true
			}
		}
	}
	return nil
}

type KeepSessionAliveAction struct{}

func (self *KeepSessionAliveAction) Execute(ctx *tutorial.ActionContext) error {
	interval := time.Minute
	if val, ok := ctx.Headers["interval"]; ok {
		if d, err := time.ParseDuration(val); err != nil {
			return err
		} else {
			interval = d
		}
	}
	fmt.Printf("Running session refresh every %v\n", interval)
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				_, _, _ = listEntitiesOfType("edge-routers", url.Values{}, false, nil, 10, false)
			}
		}
	}()
	return nil
}

type edgeRouter struct {
	id   string
	name string
}

type SelectEdgeRouterAction struct{}

func (self *SelectEdgeRouterAction) Execute(ctx *tutorial.ActionContext) error {
	for {
		err := self.SelectEdgeRouter(ctx)
		if err == nil {
			return nil
		}

		retry, err2 := tutorial.AskYesNoWithDefault(fmt.Sprintf("Error getting edge router (err=%v). Try again? [Y/N] (default Y): ", err), true)
		if err2 != nil {
			fmt.Printf("encountered error prompting for input: %v\n", err2)
			return err
		}

		if !retry {
			return err
		}
	}
}

func (self *SelectEdgeRouterAction) SelectEdgeRouter(ctx *tutorial.ActionContext) error {
	fmt.Println("")

	valid := false
	var edgeRouterName string

	for !valid {
		children, _, err := listEntitiesWithFilter("edge-routers", "limit none")
		if err != nil {
			return errors.Wrap(err, "unable to list edge routers")
		}

		var ers []*edgeRouter

		if len(children) == 0 {
			return errors.New("no edge routers found")
		}

		for _, child := range children {
			isOnline := child.S("isOnline").Data().(bool)
			if isOnline {
				id := child.S("id").Data().(string)
				name := child.S("name").Data().(string)
				ers = append(ers, &edgeRouter{id: id, name: name})
			}
		}

		if len(ers) == 0 {
			fmt.Println("Error: no online edge routers found. Found these offline edge routers: ")
			for _, child := range children {
				id := child.S("id").Data().(string)
				name := child.S("name").Data().(string)
				fmt.Printf("id: %10v name: %10v\n", id, name)
			}
			return errors.New("no on-line edge routers")
		}

		fmt.Printf("Available edge routers: \n\n")
		for idx, er := range ers {
			fmt.Printf("  %v: %v\n", idx+1, er.name)
		}
		fmt.Print("  R: Refresh list from controller\n")
		var val string

		if !ctx.Runner.AssumeDefault {
			val, err = term.Prompt("\nSelect edge router, by number or name (default 1): ")
			if err != nil {
				return err
			}
		}

		if val == "" {
			edgeRouterName = ers[0].name
			valid = true
		} else {
			if idx, err := strconv.Atoi(val); err == nil {
				if idx > 0 && idx <= len(ers) {
					edgeRouterName = ers[idx-1].name
					valid = true
				}
			}
		}

		if !valid {
			for _, er := range ers {
				if val == er.name {
					edgeRouterName = val
					valid = true
				}
			}
		}

		if !valid {
			if strings.EqualFold("r", val) {
				fmt.Println("Refreshing edge router list")
			} else {
				fmt.Printf("Invalid input %v\n", val)
			}
		}
	}

	ctx.Runner.AddVariable("edgeRouterName", edgeRouterName)
	return nil
}

type PlainEchoServerActions struct {
	server plainEchoServer
}

func (self *PlainEchoServerActions) Start(ctx *tutorial.ActionContext) error {
	if !ctx.Runner.AssumeDefault {
		start, err := tutorial.AskYesNoWithDefault("Start plain-echo-server? [Y/N] (default Y): ", true)
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

func (self *PlainEchoServerActions) Stop(ctx *tutorial.ActionContext) error {
	if !ctx.Runner.AssumeDefault {
		start, err := tutorial.AskYesNoWithDefault("Stop plain-echo-server? [Y/N] (default Y): ", true)
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

func (self *ZitiEchoServerActions) Start(ctx *tutorial.ActionContext) error {
	logrus.SetLevel(logrus.WarnLevel)

	self.server.identityJson = "echo-server.json"
	if !ctx.Runner.AssumeDefault {
		start, err := tutorial.AskYesNoWithDefault("Start ziti-echo-server? [Y/N] (default Y): ", true)
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

func (self *ZitiEchoServerActions) Stop(*tutorial.ActionContext) error {
	if self.server.listener != nil {
		return self.server.stop()
	}
	return nil
}
