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
	"context"
	goflag "flag"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	edgeSubCmd "github.com/openziti/ziti/controller/subcmd"
	"github.com/openziti/ziti/ziti/cmd/ascode/importer"
	"github.com/openziti/ziti/ziti/cmd/ops"
	"github.com/openziti/ziti/ziti/cmd/ops/database"
	"github.com/openziti/ziti/ziti/cmd/ops/verify"
	"github.com/openziti/ziti/ziti/enroll"
	"github.com/openziti/ziti/ziti/run"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/openziti/cobra-to-md"
	"github.com/openziti/ziti/ziti/cmd/agentcli"
	"github.com/openziti/ziti/ziti/cmd/ascode/exporter"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/create"
	"github.com/openziti/ziti/ziti/cmd/demo"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/cmd/fabric"
	"github.com/openziti/ziti/ziti/cmd/pki"
	"github.com/openziti/ziti/ziti/cmd/templates"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/internal/log"
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
	layout := 1

	if cliVersion := os.Getenv("ZITI_CLI_LAYOUT"); cliVersion == "2" {
		layout = 2
	} else if cliVersion == "1" {
		layout = 1
	} else {
		if cfg, _, _ := util.LoadRestClientConfig(); cfg != nil && cfg.Layout >= 1 && cfg.Layout <= 2 {
			layout = cfg.Layout
		}
	}

	if layout == 1 {
		return NewV1CmdRoot(in, out, err, cmd)
	} else {
		return NewV2CmdRoot(in, out, err, cmd)
	}
}

func NewV1CmdRoot(in io.Reader, out, err io.Writer, cmd *cobra.Command) *cobra.Command {
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

	p := common.NewOptionsProvider(out, err)

	controllerCmd := NewControllerCmd()
	routerCmd := NewRouterCmd()
	tunnelCmd := tunnel.NewTunnelCmd(true)

	createCommands := create.NewCmdCreate(out, err)
	agentCommands := agentcli.NewAgentCmd(p)
	pkiCommands := pki.NewCmdPKI(out, err)
	fabricCommand := fabric.NewFabricCmd(p)
	edgeCommand := edge.NewCmdEdge(out, err, p)
	edgeCommand.AddCommand(run.NewQuickStartCmd(out, err, context.Background()))

	demoCmd := demo.NewDemoCmd(p)
	enrollCmd := enroll.NewEnrollCmd(p)
	enrollCmd.Hidden = true

	runCmd := run.NewRunCmd(out, err)
	runCmd.Hidden = true

	opsCommands := &cobra.Command{
		Use:   "ops",
		Short: "Various utilities useful when operating a Ziti network",
	}

	opsCommands.AddCommand(database.NewCmdDb(out, err))
	opsCommands.AddCommand(fabric.NewClusterCmd(p))
	opsCommands.AddCommand(ops.NewCmdLogFormat(out, err))
	opsCommands.AddCommand(ops.NewUnwrapIdentityFileCommand(out, err))
	opsCommands.AddCommand(verify.NewVerifyCommand(out, err, context.Background()))
	opsCommands.AddCommand(exporter.NewExportCmd(out, err))
	opsCommands.AddCommand(importer.NewImportCmd(out, err))

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
				runCmd,
				enrollCmd,
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
				NewDumpCliCmd(),
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

	cmd.AddCommand(gendoc.NewGendocCmd(cmd))
	cmd.AddCommand(newCommandTreeCmd())

	return cmd
}

func NewV2CmdRoot(in io.Reader, out, err io.Writer, cmd *cobra.Command) *cobra.Command {
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

	p := common.NewOptionsProvider(out, err)

	controllerCmd := NewControllerCmd()
	controllerCmd.Hidden = true

	routerCmd := NewRouterCmd()
	routerCmd.Hidden = true

	tunnelCmd := tunnel.NewTunnelCmd(true)
	tunnelCmd.Hidden = true

	createCommands := create.NewCmdCreate(out, err)
	agentCommands := agentcli.NewAgentCmd(p)
	pkiCommands := pki.NewCmdPKI(out, err)
	fabricCommand := fabric.NewFabricCmd(p)
	edgeCommand := edge.NewCmdEdge(out, err, p)
	edgeCommand.AddCommand(run.NewQuickStartCmd(out, err, context.Background()))

	demoCmd := demo.NewDemoCmd(p)
	enrollCmd := enroll.NewEnrollCmd(p)
	runCmd := run.NewRunCmd(out, err)

	opsCommands := &cobra.Command{
		Use:   "ops",
		Short: "Various utilities useful when operating a Ziti network",
	}

	opsCommands.AddCommand(database.NewCmdDb(out, err))
	opsCommands.AddCommand(fabric.NewClusterCmd(p))
	opsCommands.AddCommand(ops.NewCmdLogFormat(out, err))
	opsCommands.AddCommand(ops.NewUnwrapIdentityFileCommand(out, err))
	opsCommands.AddCommand(verify.NewVerifyCommand(out, err, context.Background()))
	opsCommands.AddCommand(exporter.NewExportCmd(out, err))
	opsCommands.AddCommand(importer.NewImportCmd(out, err))

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
				runCmd,
				enrollCmd,
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
				NewDumpCliCmd(),
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

	cmd.AddCommand(gendoc.NewGendocCmd(cmd))
	cmd.AddCommand(newCommandTreeCmd())

	return cmd
}

func NewControllerCmd() *cobra.Command {
	var verbose, cliAgentEnabled bool
	var cliAgentAddr, cliAgentAlias, logFormatter string

	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Ziti Controller",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose {
				logrus.SetLevel(logrus.DebugLevel)
			}

			switch logFormatter {
			case "pfxlog":
				pfxlog.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").StartingToday()))
			case "json":
				pfxlog.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
			case "text":
				pfxlog.SetFormatter(&logrus.TextFormatter{})
			default:
				// let logrus do its own thing
			}

			util.LogReleaseVersionCheck()
		},
	}

	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.PersistentFlags().BoolVarP(&cliAgentEnabled, "cliagent", "a", true, "Enable/disabled CLI Agent (enabled by default)")
	cmd.PersistentFlags().StringVar(&cliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should list (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")
	cmd.PersistentFlags().StringVar(&cliAgentAlias, "cli-agent-alias", "", "Alias which can be used by ziti agent commands to find this instance")
	cmd.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")

	runCtrlCmd := run.NewRunControllerCmd()
	runCtrlCmd.Use = "run <config>"
	cmd.AddCommand(runCtrlCmd)
	cmd.AddCommand(database.NewDeleteSessionsFromConfigCmd())
	cmd.AddCommand(database.NewDeleteSessionsFromDbCmd())

	versionCmd := common.NewVersionCmd()
	versionCmd.Hidden = true
	versionCmd.Deprecated = "use 'ziti version' instead of 'ziti controller version'"
	cmd.AddCommand(versionCmd)

	edgeSubCmd.AddCommands(cmd, version.GetCmdBuildInfo())

	return cmd
}

func NewRouterCmd() *cobra.Command {
	var verbose, cliAgentEnabled bool
	var cliAgentAddr, cliAgentAlias, logFormatter string

	cmd := &cobra.Command{
		Use:   "router",
		Short: "Ziti Router",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verbose {
				logrus.SetLevel(logrus.DebugLevel)
			}

			switch logFormatter {
			case "pfxlog":
				pfxlog.SetFormatter(pfxlog.NewFormatter(pfxlog.DefaultOptions().SetTrimPrefix("github.com/openziti/").StartingToday()))
			case "json":
				pfxlog.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02T15:04:05.000Z"})
			case "text":
				pfxlog.SetFormatter(&logrus.TextFormatter{})
			default:
				// let logrus do its own thing
			}

			util.LogReleaseVersionCheck()
		},
	}

	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	cmd.PersistentFlags().BoolVar(&cliAgentEnabled, "cliagent", true, "Enable/disabled CLI Agent (enabled by default)")
	cmd.PersistentFlags().StringVar(&cliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should list (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")
	cmd.PersistentFlags().StringVar(&cliAgentAlias, "cli-agent-alias", "", "Alias which can be used by ziti agent commands to find this instance")
	cmd.PersistentFlags().StringVar(&logFormatter, "log-formatter", "", "Specify log formatter [json|pfxlog|text]")

	runRouterCmd := run.NewRunRouterCmd()
	runRouterCmd.Use = "run <config>"

	cmd.AddCommand(runRouterCmd)
	cmd.AddCommand(enroll.NewEnrollEdgeRouterCmd())

	versionCmd := common.NewVersionCmd()
	versionCmd.Hidden = true
	versionCmd.Deprecated = "use 'ziti version' instead of 'ziti router version'"
	cmd.AddCommand(versionCmd)

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

func newCommandTreeCmd() *cobra.Command {
	action := &commandTreeAction{}

	result := &cobra.Command{
		Use:   "command-tree",
		Short: "export the tree of ziti sub-commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			action.printCommandAndChildren(cmd.Root())
			return nil
		},
	}
	result.Hidden = true
	result.Flags().BoolVar(&action.showHelp, "show-help", false, "include help text in output")
	return result
}

type commandTreeAction struct {
	showHelp bool
}

func (self *commandTreeAction) printCommandAndChildren(cmd *cobra.Command) {
	hidden := ""
	if cmd.Hidden {
		hidden = " (hidden)"
	}
	fmt.Printf("%s %s\n", cmd.CommandPath(), hidden)
	if self.showHelp {
		fmt.Println("-----------------------------------------------------------------")
		_ = cmd.Help()
		fmt.Println("")
	}
	for _, child := range cmd.Commands() {
		self.printCommandAndChildren(child)
	}
}
