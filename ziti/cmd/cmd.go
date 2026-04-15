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
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/pfxlog"
	edgeSubCmd "github.com/openziti/ziti/v2/controller/subcmd"
	"github.com/openziti/ziti/v2/ziti/cmd/ascode/importer"
	"github.com/openziti/ziti/v2/ziti/cmd/ops"
	"github.com/openziti/ziti/v2/ziti/cmd/ops/database"
	"github.com/openziti/ziti/v2/ziti/cmd/ops/verify"
	ext_jwt_signer "github.com/openziti/ziti/v2/ziti/cmd/ops/verify/ext-jwt-signer"
	"github.com/openziti/ziti/v2/ziti/enroll"
	"github.com/openziti/ziti/v2/ziti/run"
	"github.com/sirupsen/logrus"

	"github.com/openziti/cobra-to-md"
	"github.com/openziti/ziti/v2/ziti/cmd/agentcli"
	"github.com/openziti/ziti/v2/ziti/cmd/ascode/exporter"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/openziti/ziti/v2/ziti/cmd/create"
	"github.com/openziti/ziti/v2/ziti/cmd/demo"
	"github.com/openziti/ziti/v2/ziti/cmd/edge"
	"github.com/openziti/ziti/v2/ziti/cmd/fabric"
	"github.com/openziti/ziti/v2/ziti/cmd/lets_encrypt"
	"github.com/openziti/ziti/v2/ziti/cmd/pki"
	"github.com/openziti/ziti/v2/ziti/cmd/templates"
	c "github.com/openziti/ziti/v2/ziti/constants"
	"github.com/openziti/ziti/v2/ziti/internal/log"
	"github.com/openziti/ziti/v2/ziti/tunnel"
	"github.com/openziti/ziti/v2/ziti/util"

	"github.com/openziti/ziti/v2/common/version"
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

var rootCommand = RootCmd{
	cobraCommand: &cobra.Command{
		Use:   "ziti",
		Short: "ziti is a CLI for working with Ziti",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.SilenceUsage = true
		},
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
		showLayoutNoticeOnce(err)
		return NewV2CmdRoot(in, out, err, cmd)
	}
}

// showLayoutNoticeOnce prints the layout 2 experimental notice to stderr once,
// then persists a flag so it is not shown again until the layout is changed.
func showLayoutNoticeOnce(errOut io.Writer) {
	cfg, _, loadErr := util.LoadRestClientConfig()
	if loadErr != nil || cfg == nil {
		return
	}

	if cfg.LayoutNoticeShown {
		return
	}

	_, _ = fmt.Fprintln(errOut, "NOTE: CLI layout 2 is experimental and will likely change before being finalized.")
	_, _ = fmt.Fprintln(errOut, "")

	cfg.LayoutNoticeShown = true
	_ = util.PersistRestClientConfig(cfg)
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

	// Build consolidated create command with edge+fabric entities and non-conflicting PKI commands
	createCommands := &cobra.Command{
		Use:   "create",
		Short: "creates various entities managed by the Ziti Controller",
	}
	edge.AddCreateCommandsConsolidated(createCommands, out, err)
	fabric.AddCreateCommandsConsolidated(createCommands, p)

	createCommands.AddCommand(pki.NewCmdPKICreateIntermediate(out, err))
	createCommands.AddCommand(pki.NewCmdPKICreateServer(out, err))
	createCommands.AddCommand(pki.NewCmdPKICreateClient(out, err))
	createCommands.AddCommand(pki.NewCmdPKICreateCSR(out, err))

	// Add config file generators as children of the edge config command
	configFileChildren := create.NewCmdCreateConfig()
	if edgeConfigCmd := findSubCommand(createCommands, "config"); edgeConfigCmd != nil {
		for _, child := range configFileChildren.Commands() {
			edgeConfigCmd.AddCommand(child)
		}
	}

	topLevelDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "deletes various entities managed by the Ziti Controller",
	}
	edge.AddDeleteCommandsConsolidated(topLevelDeleteCmd, out, err)
	fabric.AddDeleteCommandsConsolidated(topLevelDeleteCmd, p)

	topLevelListCmd := &cobra.Command{
		Use:     "list",
		Short:   "lists various entities managed by the Ziti Controller",
		Aliases: []string{"ls"},
	}
	edge.AddListCommandsConsolidated(topLevelListCmd, out, err)
	fabric.AddListCommandsConsolidated(topLevelListCmd, p)

	topLevelUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "updates various entities managed by the Ziti Controller",
	}
	edge.AddUpdateCommandsConsolidated(topLevelUpdateCmd, out, err)
	fabric.AddUpdateCommandsConsolidated(topLevelUpdateCmd, p)

	loginCmd := edge.NewLoginCmd(out, err)
	forgetCmd := edge.NewLogoutCmd(out, err)
	forgetCmd.Use = "forget"
	forgetCmd.Short = "removes stored credentials for a given identity"
	loginCmd.AddCommand(forgetCmd)
	loginCmd.AddCommand(edge.NewUseCmd(out, err))

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
				topLevelDeleteCmd,
				topLevelListCmd,
				topLevelUpdateCmd,
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
			Message: "Session Management",
			Commands: []*cobra.Command{
				loginCmd,
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
	cmd.AddCommand(NewCliCmd(out, err))


	return cmd
}

func NewV2CmdRoot(in io.Reader, out, err io.Writer, cmd *cobra.Command) *cobra.Command {
	// Disable automatic completion command - we provide our own under ops tools
	cmd.CompletionOptions.DisableDefaultCmd = true

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

	// Keep pki command hidden for backward compatibility
	pkiCommands := pki.NewCmdPKI(out, err)
	pkiCommands.Hidden = true

	// Create setup command for deployment/setup tasks
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Setup Ziti components and infrastructure",
	}

	// setup controller - controller setup commands
	setupControllerCmd := &cobra.Command{
		Use:   "controller",
		Short: "Setup controller components",
	}
	controllerConfigCmd := create.NewCmdCreateConfigController().Command
	controllerConfigCmd.Use = "config"
	controllerConfigCmd.Short = "Create a controller config file"
	setupControllerCmd.AddCommand(controllerConfigCmd)

	controllerDbCmd := edgeSubCmd.NewEdgeInitializeCmd(version.GetCmdBuildInfo())
	controllerDbCmd.Use = "database <config-file>"
	controllerDbCmd.Short = "Initialize the controller database"
	setupControllerCmd.AddCommand(controllerDbCmd)
	setupCmd.AddCommand(setupControllerCmd)

	// setup router - router setup commands
	setupRouterCmd := &cobra.Command{
		Use:   "router",
		Short: "Setup router components",
	}
	routerConfigCmd := create.NewCmdCreateConfigRouter(nil).Command
	routerConfigCmd.Use = "config"
	routerConfigCmd.Short = "Create a router config file"
	setupRouterCmd.AddCommand(routerConfigCmd)
	setupCmd.AddCommand(setupRouterCmd)

	// setup environment - generate environment file
	setupCmd.AddCommand(create.NewCmdCreateConfigEnvironment())

	// setup pki - PKI creation commands
	setupPkiCmd := pki.NewCmdPKICreate(out, err)
	setupPkiCmd.Use = "pki"
	setupPkiCmd.Short = "Create PKI artifacts (certificates, keys, CAs)"
	setupCmd.AddCommand(setupPkiCmd)

	demoCmd := demo.NewDemoCmd(p)
	enrollCmd := enroll.NewEnrollCmdV2(p)
	// Add reenroll-router to enroll command
	reenrollRouterCmd := edge.NewReEnrollEdgeRouterCmd(out, err)
	reenrollRouterCmd.Use = "reenroll-router <idOrName>"
	reenrollRouterCmd.Short = "re-enroll an edge router"
	enrollCmd.AddCommand(reenrollRouterCmd)
	runCmd := run.NewRunCmd(out, err)

	opsCommands := &cobra.Command{
		Use:   "ops",
		Short: "Various utilities useful when operating a Ziti network",
	}

	dbCmd := database.NewCmdDb(out, err)
	// Add fabric db commands to ops db for consolidated access
	dbCmd.AddCommand(fabric.NewDbSnapshotCmd(p))
	dbCmd.AddCommand(fabric.NewDbCheckIntegrityCmd(p))
	dbCmd.AddCommand(fabric.NewDbCheckIntegrityStatusCmd(p))
	opsCommands.AddCommand(dbCmd)
	opsCommands.AddCommand(fabric.NewClusterCmd(p))

	// Group utility tools under ops tools
	toolsCmd := &cobra.Command{
		Use:   "tools",
		Short: "miscellaneous utility tools",
	}
	toolsCmd.AddCommand(ops.NewCmdLogFormat(out, err))
	toolsCmd.AddCommand(ops.NewUnwrapIdentityFileCommand(out, err))
	toolsCmd.AddCommand(newCompletionCmd())
	toolsCmd.AddCommand(lets_encrypt.NewCmdLE(out, err))
	opsCommands.AddCommand(toolsCmd)

	opsCommands.AddCommand(exporter.NewExportCmd(out, err))
	opsCommands.AddCommand(importer.NewImportCmd(out, err))

	// Add agent under ops
	opsCommands.AddCommand(agentcli.NewAgentCmd(p))

	// Add inspect under ops
	opsCommands.AddCommand(fabric.NewInspectCmd(p))

	// Add stream under ops
	opsCommands.AddCommand(fabric.NewStreamCommand(p))

	// Add trace under ops
	opsCommands.AddCommand(edge.NewTraceCmd(out, err))

	// Consolidate validate commands under ops
	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "validate model data",
	}
	validateCmd.AddCommand(fabric.NewValidateCircuitsCmd(p))
	validateCmd.AddCommand(fabric.NewValidateTerminatorsCmd(p))
	validateCmd.AddCommand(fabric.NewValidateRouterLinksCmd(p))
	validateCmd.AddCommand(fabric.NewValidateRouterSdkTerminatorsCmd(p))
	validateCmd.AddCommand(fabric.NewValidateRouterErtTerminatorsCmd(p))
	validateCmd.AddCommand(fabric.NewValidateRouterDataModelCmd(p))
	validateCmd.AddCommand(fabric.NewValidateIdentityConnectionStatusesCmd(p))
	validateCmd.AddCommand(edge.NewValidateServiceHostingCmd(p))
	opsCommands.AddCommand(validateCmd)

	// Create top-level verify command
	verifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "verify network configuration, policies, and connectivity",
	}
	// Rename policy-advisor to policy
	policyCmd := edge.NewPolicyAdvisorCmd(out, err)
	policyCmd.Use = "policy"
	policyCmd.Short = "verify policies between identities and services"
	verifyCmd.AddCommand(policyCmd)
	verifyCmd.AddCommand(edge.NewTraceRouteCmd(out, err))
	verifyCmd.AddCommand(edge.NewVerifyCaCmd(out, err))
	verifyCmd.AddCommand(verify.NewVerifyNetwork(out, err))
	verifyCmd.AddCommand(verify.NewVerifyTraffic(out, err))
	verifyCmd.AddCommand(ext_jwt_signer.NewVerifyExtJwtSignerCmd(out, err, context.Background()))

	// Create top-level get command
	getCmd := edge.NewShowCmd(out, err)
	getCmd.Use = "get"
	getCmd.Short = "gets various entities managed by the Ziti Edge Controller"
	// Add controller version under get
	controllerVersionCmd := edge.NewVersionCmd(out, err)
	controllerVersionCmd.Use = "controller-version"
	controllerVersionCmd.Short = "shows the version of the Ziti controller"
	getCmd.AddCommand(controllerVersionCmd)

	// Create top-level login command
	loginCmd := edge.NewLoginCmd(out, err)
	forgetCmd := edge.NewLogoutCmd(out, err)
	forgetCmd.Use = "forget"
	forgetCmd.Short = "removes stored credentials for a given identity"
	loginCmd.AddCommand(forgetCmd)
	loginCmd.AddCommand(edge.NewUseCmd(out, err))

	// Create top-level CRUD commands that combine edge and fabric subcommands
	// - Edge commands are the default for most entities
	// - Fabric service/router commands are hidden (edge versions are preferred)
	// - Fabric terminator is the default (edge terminator is excluded)
	topLevelCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "creates various entities managed by the Ziti Controller",
	}
	edge.AddCreateCommandsConsolidated(topLevelCreateCmd, out, err)
	fabric.AddCreateCommandsConsolidated(topLevelCreateCmd, p)

	topLevelDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "deletes various entities managed by the Ziti Controller",
	}
	edge.AddDeleteCommandsConsolidated(topLevelDeleteCmd, out, err)
	fabric.AddDeleteCommandsConsolidated(topLevelDeleteCmd, p)

	topLevelListCmd := &cobra.Command{
		Use:     "list",
		Short:   "lists various entities managed by the Ziti Controller",
		Aliases: []string{"ls"},
	}
	edge.AddListCommandsConsolidated(topLevelListCmd, out, err)
	fabric.AddListCommandsConsolidated(topLevelListCmd, p)

	topLevelUpdateCmd := &cobra.Command{
		Use:   "update",
		Short: "updates various entities managed by the Ziti Controller",
	}
	edge.AddUpdateCommandsConsolidated(topLevelUpdateCmd, out, err)
	fabric.AddUpdateCommandsConsolidated(topLevelUpdateCmd, p)

	groups := templates.CommandGroups{
		{
			Message: "Working with Ziti resources:",
			Commands: []*cobra.Command{
				topLevelCreateCmd,
				topLevelDeleteCmd,
				topLevelListCmd,
				topLevelUpdateCmd,
			},
		},
		{
			Message: "Executing Ziti components:",
			Commands: []*cobra.Command{
				runCmd,
				enrollCmd,
				setupCmd,
				controllerCmd,
				routerCmd,
				tunnelCmd,
				pkiCommands,
			},
		},
		{
			Message: "Interacting with the Ziti controller",
			Commands: []*cobra.Command{
				getCmd,
			},
		},
		{
			Message: "Session Management",
			Commands: []*cobra.Command{
				loginCmd,
			},
		},
		{
			Message: "Utilities",
			Commands: []*cobra.Command{
				opsCommands,
				verifyCmd,
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
	versionCmd := common.NewVersionCmd()
	versionCmd.Hidden = true
	cmd.AddCommand(versionCmd)

	cmd.AddCommand(gendoc.NewGendocCmd(cmd))
	cmd.AddCommand(newCommandTreeCmd())
	cmd.AddCommand(NewCliCmd(out, err))


	// Add hidden root-level aliases for power users
	hiddenAgentCmd := agentcli.NewAgentCmd(p)
	hiddenAgentCmd.Hidden = true
	cmd.AddCommand(hiddenAgentCmd)

	hiddenLogFormatCmd := ops.NewCmdLogFormat(out, err)
	hiddenLogFormatCmd.Hidden = true
	cmd.AddCommand(hiddenLogFormatCmd)

	addV1BackwardCompatCommands(cmd, out, err, p, topLevelCreateCmd, enrollCmd, opsCommands)

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
	cmd.PersistentFlags().StringVar(&cliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should listen (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")
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
	cmd.PersistentFlags().StringVar(&cliAgentAddr, "cli-agent-addr", "", "Specify where CLI Agent should listen (ex: unix:/tmp/myfile.sock or tcp:127.0.0.1:10001)")
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

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for Ziti CLI.

To load completions:

Bash:
  $ source <(ziti ops tools completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ ziti ops tools completion bash > /etc/bash_completion.d/ziti
  # macOS:
  $ ziti ops tools completion bash > $(brew --prefix)/etc/bash_completion.d/ziti

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc
  # To load completions for each session, execute once:
  $ ziti ops tools completion zsh > "${fpath[1]}/_ziti"

Fish:
  $ ziti ops tools completion fish | source
  # To load completions for each session, execute once:
  $ ziti ops tools completion fish > ~/.config/fish/completions/ziti.fish

PowerShell:
  PS> ziti ops tools completion powershell | Out-String | Invoke-Expression
  # To load completions for every new session, run:
  PS> ziti ops tools completion powershell > ziti.ps1
  # and source this file from your PowerShell profile.
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
	return cmd
}

// findSubCommand returns the first direct child of parent whose Name() matches,
// or nil if none is found.
func findSubCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

// addV1BackwardCompatCommands adds hidden V1 command paths to the V2 CLI layout
// so that existing scripts and muscle memory still work.
func addV1BackwardCompatCommands(
	cmd *cobra.Command,
	out, errOut io.Writer,
	p common.OptionsProvider,
	topLevelCreateCmd *cobra.Command,
	enrollCmd *cobra.Command,
	opsCommands *cobra.Command,
) {
	// 1. ziti edge (full V1 edge command tree)
	hiddenEdge := edge.NewCmdEdge(out, errOut, p)
	hiddenEdge.Hidden = true
	hiddenEdge.AddCommand(run.NewQuickStartCmd(out, errOut, context.Background()))
	cmd.AddCommand(hiddenEdge)

	// 2. ziti fabric (full V1 fabric command tree)
	hiddenFabric := fabric.NewFabricCmd(p)
	hiddenFabric.Hidden = true
	cmd.AddCommand(hiddenFabric)

	// 3. ziti create <pki commands> (V1 had PKI commands directly under create)
	// Skip 'ca' because it conflicts with the edge CA create command already at that path
	hiddenClient := pki.NewCmdPKICreateClient(out, errOut)
	hiddenClient.Hidden = true
	topLevelCreateCmd.AddCommand(hiddenClient)

	hiddenServer := pki.NewCmdPKICreateServer(out, errOut)
	hiddenServer.Hidden = true
	topLevelCreateCmd.AddCommand(hiddenServer)

	hiddenIntermediate := pki.NewCmdPKICreateIntermediate(out, errOut)
	hiddenIntermediate.Hidden = true
	topLevelCreateCmd.AddCommand(hiddenIntermediate)

	hiddenCSR := pki.NewCmdPKICreateCSR(out, errOut)
	hiddenCSR.Hidden = true
	topLevelCreateCmd.AddCommand(hiddenCSR)

	// ziti create config {controller,router,environment} (V1 config file generators)
	// V2 already has "ziti create config" for edge config entities, so we can't
	// add a second "config" parent. Instead, add the V1 children directly to the
	// existing edge config command.
	configFileChildren := create.NewCmdCreateConfig()
	if edgeConfigCmd := findSubCommand(topLevelCreateCmd, "config"); edgeConfigCmd != nil {
		for _, child := range configFileChildren.Commands() {
			child.Hidden = true
			edgeConfigCmd.AddCommand(child)
		}
	}

	// 4. ziti completion (V1 had completion at root level)
	hiddenCompletion := newCompletionCmd()
	hiddenCompletion.Hidden = true
	cmd.AddCommand(hiddenCompletion)

	// 5. ziti enroll edge-router (V1 path, now 'ziti enroll router')
	hiddenEnrollEdgeRouter := enroll.NewEnrollEdgeRouterCmd()
	hiddenEnrollEdgeRouter.Use = "edge-router"
	hiddenEnrollEdgeRouter.Hidden = true
	enrollCmd.AddCommand(hiddenEnrollEdgeRouter)

	// 6. ziti ops log-format and ziti ops unwrap (V1 paths, now under ops tools)
	hiddenLogFormat := ops.NewCmdLogFormat(out, errOut)
	hiddenLogFormat.Hidden = true
	opsCommands.AddCommand(hiddenLogFormat)

	hiddenUnwrap := ops.NewUnwrapIdentityFileCommand(out, errOut)
	hiddenUnwrap.Hidden = true
	opsCommands.AddCommand(hiddenUnwrap)

	// 7. ziti ops verify (V1 path, now top-level 'ziti verify')
	hiddenVerify := verify.NewVerifyCommand(out, errOut, context.Background())
	hiddenVerify.Hidden = true
	opsCommands.AddCommand(hiddenVerify)
}
