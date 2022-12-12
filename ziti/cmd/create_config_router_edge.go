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
	_ "embed"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	optionWSS               = "wss"
	defaultWSS              = false
	wssDescription          = "Create an edge router config with wss enabled"
	optionPrivate           = "private"
	defaultPrivate          = false
	privateDescription      = "Create a private router config"
	tproxyTunMode           = "tproxy"
	hostTunMode             = "host"
	noneTunMode             = "none"
	optionTunnelerMode      = "tunnelerMode"
	defaultTunnelerMode     = hostTunMode
	tunnelerModeDescription = "Specify tunneler mode \"" + noneTunMode + "\", \"" + hostTunMode + "\", or \"" + tproxyTunMode + "\""
	optionLanInterface      = "lanInterface"
	defaultLanInterface     = ""
	lanInterfaceDescription = "The interface on host of the router to insert iptables ingress filter rules"
)

var (
	createConfigRouterEdgeLong = templates.LongDesc(`
		Creates the edge router config
`)

	createConfigRouterEdgeExample = templates.Examples(`
		# Create the edge router config for a router named my_router
		ziti create config router edge --routerName my_router
	`)
)

//go:embed config_templates/router.yml
var routerConfigEdgeTemplate string

// NewCmdCreateConfigRouterEdge creates a command object for the "edge" command
func NewCmdCreateConfigRouterEdge() *cobra.Command {

	cmd := &cobra.Command{
		Use:     "edge",
		Short:   "Create an edge router config",
		Aliases: []string{"edge"},
		Long:    createConfigRouterEdgeLong,
		Example: createConfigRouterEdgeExample,
		PreRun: func(cmd *cobra.Command, args []string) {
			data.Router.IsWss = routerOptions.WssEnabled
			data.Router.IsPrivate = routerOptions.IsPrivate
			data.Router.TunnelerMode = routerOptions.TunnelerMode
			data.Router.Edge.LanInterface = routerOptions.LanInterface
		},
		Run: func(cmd *cobra.Command, args []string) {
			routerOptions.Cmd = cmd
			routerOptions.Args = args
			err := routerOptions.runEdgeRouter(data)
			cmdhelper.CheckErr(err)
		},
	}

	routerOptions.addCreateFlags(cmd)
	routerOptions.addEdgeFlags(cmd)

	return cmd
}

func (options *CreateConfigRouterOptions) addEdgeFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&options.WssEnabled, optionWSS, defaultWSS, wssDescription)
	cmd.Flags().BoolVar(&options.IsPrivate, optionPrivate, defaultPrivate, privateDescription)
	cmd.PersistentFlags().StringVarP(&options.TunnelerMode, optionTunnelerMode, "", defaultTunnelerMode, tunnelerModeDescription)
	cmd.PersistentFlags().StringVarP(&options.LanInterface, optionLanInterface, "", defaultLanInterface, lanInterfaceDescription)
	cmd.PersistentFlags().StringVarP(&options.RouterName, optionRouterName, "n", "", "name of the router")
	err := cmd.MarkPersistentFlagRequired(optionRouterName)
	if err != nil {
		return
	}
}

// run implements the command
func (options *CreateConfigRouterOptions) runEdgeRouter(data *ConfigTemplateValues) error {
	// Ensure private and wss are not both used
	if options.IsPrivate && options.WssEnabled {
		return errors.New("Flags for private and wss configs are mutually exclusive. You must choose private or wss, not both")
	}

	// Make sure the tunneler mode is valid
	if options.TunnelerMode != hostTunMode && options.TunnelerMode != tproxyTunMode && options.TunnelerMode != noneTunMode {
		return errors.New("Unknown tunneler mode [" + options.TunnelerMode + "] provided, should be \"" + noneTunMode + "\", \"" + hostTunMode + "\", or \"" + tproxyTunMode + "\"")
	}

	tmpl, err := template.New("edge-router-config").Parse(routerConfigEdgeTemplate)
	if err != nil {
		return err
	}

	var f *os.File
	if strings.ToLower(options.Output) != "stdout" {
		// Check if the path exists, fail if it doesn't
		basePath := filepath.Dir(options.Output) + "/"
		if _, err := os.Stat(filepath.Dir(basePath)); os.IsNotExist(err) {
			return err
		}

		f, err = os.Create(options.Output)
		logrus.Debugf("Created output file: %s", options.Output)
		if err != nil {
			return errors.Wrapf(err, "unable to create config file: %s", options.Output)
		}
	} else {
		f = os.Stdout
	}
	defer func() { _ = f.Close() }()

	if err := tmpl.Execute(f, data); err != nil {
		return errors.Wrap(err, "unable to execute template")
	}

	logrus.Debugf("Edge Router configuration generated successfully and written to: %s", options.Output)

	return nil
}
