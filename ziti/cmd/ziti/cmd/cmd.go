/*
	Copyright NetFoundry, Inc.

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

	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/edge_controller"

	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	c "github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/openziti/ziti/ziti/cmd/ziti/internal/log"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"

	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/templates"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// InitOptions the flags for running init
type MainOptions struct {
	CommonOptions
}

type RootCmd struct {
	factory cmdutil.Factory

	configFile string

	RegistryPath string

	clusterName string

	cobraCommand *cobra.Command
}

var rootCommand = RootCmd{
	cobraCommand: &cobra.Command{
		Use:   "ziti",
		Short: "ziti is a CLI for working with Ziti",
		Long: `
'ziti' is a CLI for working with a Ziti deployment.
`,
	},
}

// exitWithError will terminate execution with an error result
// It prints the error to stderr and exits with a non-zero exit code
func exitWithError(err error) {
	fmt.Fprintf(os.Stderr, "\n%v\n", err)
	os.Exit(1)
}

// Execute is ...
func Execute() {
	goflag.CommandLine.Parse([]string{})
	if err := rootCommand.cobraCommand.Execute(); err != nil {
		exitWithError(err)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	factory := cmdutil.NewFactory()
	rootCommand.factory = factory
	NewCmdRoot(factory, os.Stdin, os.Stdout, os.Stderr)

}

func NewCmdRoot(f cmdutil.Factory, in io.Reader, out, err io.Writer) *cobra.Command {

	cmd := rootCommand.cobraCommand

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

	initCommands := NewCmdInit(f, out, err)
	createCommands := NewCmdCreate(f, out, err)
	updateCommands := NewCmdUpdate(f, out, err)
	deleteCommands := NewCmdDelete(f, out, err)
	executeCommands := NewCmdExecute(f, out, err)
	psCommands := NewCmdPs(f, out, err)
	pkiCommands := NewCmdPKI(f, out, err)
	edgeCommand := edge_controller.NewCmdEdge(f, out, err)
	logFilter := NewCmdLogFormat(f, out, err)

	installCommands := []*cobra.Command{
		NewCmdInstall(f, out, err),
		NewCmdUnInstall(f, out, err),
		NewCmdUpgrade(f, out, err),
	}

	groups := templates.CommandGroups{
		{
			Message:  "Installing Ziti components:",
			Commands: installCommands,
		},
		{
			Message: "Working with Ziti resources:",
			Commands: []*cobra.Command{
				initCommands,
				createCommands,
				updateCommands,
				deleteCommands,
			},
		},
		{
			Message: "Executing Ziti components:",
			Commands: []*cobra.Command{
				executeCommands,
				psCommands,
				pkiCommands,
			},
		},
		{
			Message: "Interacting with Ziti edge components",
			Commands: []*cobra.Command{
				edgeCommand,
			},
		},
		{
			Message: "Utilities",
			Commands: []*cobra.Command{
				logFilter,
			},
		},
	}

	groups.Add(cmd)

	cmd.AddCommand(NewCmdVersion(f, out, err))
	cmd.Version = version.GetVersion()
	cmd.SetVersionTemplate("{{printf .Version}}\n")
	cmd.AddCommand(NewCmdArt(f, out, err))
	cmd.AddCommand(NewCmdPlaybook(f, out, err))
	cmd.AddCommand(NewCmdPing(f, out, err))
	cmd.AddCommand(NewCmdAdhoc(f, out, err))
	cmd.AddCommand(NewCmdUse(f, out, err))

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

	rootCommand.RegistryPath = viper.GetString("ZITI_STATE_STORE")

	// Tolerate multiple slashes at end
	rootCommand.RegistryPath = strings.TrimSuffix(rootCommand.RegistryPath, "/")
}
