package edge

import (
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"strings"
)

// Options are common options for edge controller commands
type zitiEchoClientOptions struct {
	common.CommonOptions
	identity string
}

func (self *zitiEchoClientOptions) run() error {
	logrus.SetLevel(logrus.WarnLevel)
	echoClient, err := NewZitiEchoClient(self.identity)
	if err != nil {
		return err
	}
	return echoClient.echo(strings.Join(self.Args, " "))
}

func newZitiEchoClientCmd(p common.OptionsProvider) *cobra.Command {
	options := &zitiEchoClientOptions{
		CommonOptions: p(),
	}

	cmd := &cobra.Command{
		Use:   "ziti-echo-client strings to echo",
		Short: "Runs a ziti-enabled http echo client",
		Args:  cobra.MinimumNArgs(1),
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
	cmd.Flags().StringVar(&options.identity, "identity", "echo-client.json", "Specify the config file to use")

	return cmd
}
