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

package cmd

import (
	goflag "flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/openziti/ziti/ziti/cmd/agentcli"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/create"
	"github.com/openziti/ziti/ziti/cmd/database"
	"github.com/openziti/ziti/ziti/cmd/demo"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/cmd/fabric"
	"github.com/openziti/ziti/ziti/cmd/pki"
	"github.com/openziti/ziti/ziti/cmd/templates"
	"github.com/openziti/ziti/ziti/cmd/verify"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/controller"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/openziti/ziti/ziti/router"
	"github.com/openziti/ziti/ziti/tunnel"
	"github.com/openziti/ziti/ziti/util"

	"github.com/openziti/ziti/common/version"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// InitOptions the flags for running init
type MainOptions struct {
	common.CommonOptions
}

type RootCmd struct {
	configFile string

	cobraCommand *cobra.Command
}

func GetRootCommand() *cobra.Command {
	return rootCommand.cobraCommand
}

var rootCommand = RootCmd{
	cobraCommand: &cobra.Command{
		Use:   "ziti",
		Short: "ziti is a CLI for working with Ziti",
		Long: `
'ziti' is a CLI for working with a Ziti deployment.
`},
}

// exitWithError will terminate execution with an error result
// It prints the error to stderr and exits with a non-zero exit code
func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "\n%v\n", err)
	os.Exit(1)
}

// Execute is ...
func Execute() {
	if err := rootCommand.cobraCommand.Execute(); err != nil {
		exitWithError(err)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	NewCmdRoot(os.Stdin, os.Stdout, os.Stderr, rootCommand.cobraCommand)
}

func NewCmdRoot(in io.Reader, out, err io.Writer, cmd *cobra.Command) *cobra.Command {
	goflag.CommandLine.VisitAll(func(goflag *goflag.Flag) {
		switch goflag.Name {
		// Skip things that are dragged in by our dependencies
		case "alsologtostderr":
		case "logtostderr":
		case "log_backtrace_at":
		case "log_dir":
		case "stderrthreshold":
		case "vmodule":
		case "v":

		default:
			cmd.PersistentFlags().AddGoFlag(goflag)
		}
	})

	viper.SetEnvPrefix(c.ZITI) // All env vars we seek will be prefixed with "ZITI_"
	viper.AutomaticEnv()
	replacer := strings.NewReplacer("-", "_") // We use underscores in env var names, but use dashes in flag names
	viper.SetEnvKeyReplacer(replacer)

	// cmd.PersistentFlags().StringVar(&rootCommand.configFile, "config", "", "yaml config file (default is $HOME/.ziti.yaml)")
	// viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))
	// viper.SetDefault("config", "$HOME/.ziti.yaml")

	// cmd.PersistentFlags().StringVar(&rootCommand.RegistryPath, "state", "", "Location of state storage (ziti 'config' file). Overrides ZITI_STATE_STORE environment variable")
	// viper.BindPFlag("ZITI_STATE_STORE", cmd.PersistentFlags().Lookup("state"))
	// viper.BindEnv("ZITI_STATE_STORE")

	// defaultClusterName := os.Getenv("ZITI_CLUSTER_NAME")
	// cmd.PersistentFlags().StringVarP(&rootCommand.clusterName, "name", "", defaultClusterName, "Name of cluster. Overrides ZITI_CLUSTER_NAME environment variable")

	p := common.NewOptionsProvider(out, err)

	createCommands := create.NewCmdCreate(out, err)
	controllerCmd := controller.NewControllerCmd()
	tunnelCmd := tunnel.NewTunnelCmd(false)
	routerCmd := router.NewRouterCmd()
	agentCommands := agentcli.NewAgentCmd(p)
	pkiCommands := pki.NewCmdPKI(out, err)
	fabricCommand := fabric.NewFabricCmd(p)
	edgeCommand := edge.NewCmdEdge(out, err, p)
	demoCmd := demo.NewDemoCmd(p)

	opsCommands := &cobra.Command{
		Use: "ops",
		Short: "Various utilities useful when operating a Ziti network",
	}

	opsCommands.AddCommand(database.NewCmdDb(out, err))
	opsCommands.AddCommand(NewCmdLogFormat(out, err))
	opsCommands.AddCommand(NewUnwrapIdentityFileCommand(out, err))
	opsCommands.AddCommand(verify.NewVerifyNetwork())
	opsCommands.AddCommand(verify.NewVerifyTraffic())

	groups := templates.CommandGroups{
		{
			Message: "Working with Ziti resources:",
			Commands: []*cobra.Command{
				createCommands,
			},
		},
		{
			Message: "Executing Ziti components:",
			Commands: []*cobra.Command{
				agentCommands,
				controllerCmd,
				routerCmd,
				tunnelCmd,
				pkiCommands,
			},
		},
		{
			Message: "Interacting with the Ziti controller",
			Commands: []*cobra.Command{
				fabricCommand,
				edgeCommand,
			},
		},
		{
			Message: "Utilities",
			Commands: []*cobra.Command{
				opsCommands,
			},
		},
		{
			Message: "Learning Ziti",
			Commands: []*cobra.Command{
				demoCmd,
			},
		},
	}

	groups.Add(cmd)

	cmd.Version = version.GetVersion()
	cmd.SetVersionTemplate("{{printf .Version}}\n")
	cmd.AddCommand(NewCmdArt(out, err))
	cmd.AddCommand(common.NewVersionCmd())

	return cmd
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// Config file precedence: --config flag, ${HOME}/.ziti.yaml ${HOME}/.ziti/config
	configFile := rootCommand.configFile
	if configFile == "" {
		home := util.HomeDir()
		configPaths := []string{
			filepath.Join(home, ".ziti.yaml"),
			filepath.Join(home, ".ziti", "config"),
		}
		for _, p := range configPaths {
			_, err := os.Stat(p)
			if err == nil {
				configFile = p
				break
			} else if !os.IsNotExist(err) {
				log.Infof("error checking for file %s: %v", p, err)
			}
		}
	}

	if configFile != "" {
		viper.SetConfigFile(configFile)
		viper.SetConfigType("yaml")

		if err := viper.ReadInConfig(); err != nil {
			log.Warnf("error reading config: %v", err)
		}
	}
}

func NewRootCommand(in io.Reader, out, err io.Writer) *cobra.Command {
	//need to make new CMD every time because the flags are not thread safe...
	ret := &cobra.Command{
		Use:   "ziti",
		Short: "ziti is a CLI for working with Ziti",
		Long: `
'ziti' is a CLI for working with a Ziti deployment.
`}
	NewCmdRoot(in, out, err, ret)

	return ret
}
