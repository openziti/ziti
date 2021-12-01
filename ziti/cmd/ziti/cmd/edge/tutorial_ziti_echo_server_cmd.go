package edge

import (
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"time"
)

// Options are common options for edge controller commands
type zitiEchoServerOptions struct {
	common.CommonOptions
	identity string
}

func (self *zitiEchoServerOptions) run() error {
	echoServer := &zitiEchoServer{
		identityJson: self.identity,
	}
	if err := echoServer.run(); err != nil {
		return err
	}
	time.Sleep(time.Hour * 24 * 365 * 100)
	return nil
}

func newZitiEchoServerCmd(p common.OptionsProvider) *cobra.Command {
	options := &zitiEchoServerOptions{
		CommonOptions: p(),
	}

	cmd := &cobra.Command{
		Use:   "ziti-echo-server",
		Short: "Runs a ziti-based http echo service",
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
	cmd.Flags().StringVar(&options.identity, "identity", "echo-server.json", "Specify the config file to use")

	return cmd
}
