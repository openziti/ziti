package edge

import (
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"time"
)

// Options are common options for edge controller commands
type plainEchoServerOptions struct {
	common.CommonOptions
	port uint16
}

func (self *plainEchoServerOptions) run() error {
	echoServer := &plainEchoServer{
		Port: int(self.port),
	}
	if err := echoServer.run(); err != nil {
		return err
	}
	time.Sleep(time.Hour * 24 * 365 * 100)
	return nil
}

func newPlainEchoServerCmd(p common.OptionsProvider) *cobra.Command {
	options := &plainEchoServerOptions{
		CommonOptions: p(),
	}

	cmd := &cobra.Command{
		Use:   "plain-echo-server",
		Short: "Runs a simple http echo service",
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
	cmd.Flags().Uint16Var(&options.port, "port", 0, "Specify the port to listen on")

	return cmd
}
