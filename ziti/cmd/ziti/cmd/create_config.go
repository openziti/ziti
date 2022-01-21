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
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"os"
)

const (
	optionVerbose      = "verbose"
	defaultVerbose     = false
	verboseDescription = "Enable verbose logging. Logging will be sent to stdout if the config output is sent to a file. If output is sent to stdout, logging will be sent to stderr"
	optionOutput       = "output"
	defaultOutput      = "stdout"
	outputDescription  = "designated output destination for config, use \"stdout\" or a filepath."
	optionPKI          = "pkiPath"
	pkiDescription     = "Location of the public key infrastructure"
)

// CreateConfigOptions the options for the create config command
type CreateConfigOptions struct {
	common.CommonOptions

	Output       string
	DatabaseFile string
	PKIPath      string
}

type ConfigTemplateValues struct {
	ZitiPKI                      string
	ZitiHome                     string
	Hostname                     string
	ZitiFabMgmtPort              string
	ZitiSigningIntermediateName  string
	ZitiCtrlRawname              string
	ZitiEdgeCtrlIntermediateName string
	ZitiEdgeCtrlHostname         string

	Controller ControllerTemplateValues
	Router     RouterTemplateValues
}

type ControllerTemplateValues struct {
	Hostname                     string
	FabCtrlPort                  string
	EdgeCtrlAPI                  string
	EdgeCtrlPort                 string
	Listener                     string
	MgmtListener                 string
	Rawname                      string
	EdgeAPISessionTimeoutMinutes int
	WebListener                  ControllerWebListenerValues
}

type ControllerWebListenerValues struct {
	IdleTimeoutMS  int
	ReadTimeoutMS  int
	WriteTimeoutMS int
	MinTLSVersion  string
	MaxTLSVersion  string
}

type RouterTemplateValues struct {
	Name      string
	IsPrivate bool
	IsFabric  bool
	IsWss     bool
	Edge      EdgeRouterTemplateValues
	Wss       WSSRouterTemplateValues
	Forwarder RouterForwarderTemplateValues
	Listener  RouterListenerTemplateValues
}

type EdgeRouterTemplateValues struct {
	Hostname string
	Port     string
}

type WSSRouterTemplateValues struct {
	WriteTimeout     int
	ReadTimeout      int
	IdleTimeout      int
	PongTimeout      int
	PingInterval     int
	HandshakeTimeout int
	ReadBufferSize   int
	WriteBufferSize  int
}

type RouterForwarderTemplateValues struct {
	LatencyProbeInterval  int
	XgressDialQueueLength int
	XgressDialWorkerCount int
	LinkDialQueueLength   int
	LinkDialWorkerCount   int
}

type RouterListenerTemplateValues struct {
	ConnectTimeoutMs   int
	GetSessionTimeoutS int
	BindPort           int
	OutQueueSize       int
}

// NewCmdCreateConfig creates a command object for the "config" command
func NewCmdCreateConfig(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Creates a config file for specified Ziti component",
		Aliases: []string{"cfg"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdhelper.CheckErr(cmd.Help())
		},
	}

	// Get env variable data global to all config files
	templateData := &ConfigTemplateValues{}
	templateData.populateEnvVars()
	templateData.populateDefaults()

	cmd.AddCommand(NewCmdCreateConfigController(templateData))
	cmd.AddCommand(NewCmdCreateEnvironment(f, out, errOut))
	cmd.AddCommand(NewCmdCreateConfigRouter(templateData))

	return cmd
}

// Add flags that are global to all "create config" commands
func (options *CreateConfigOptions) addCreateFlags(cmd *cobra.Command) {
	// Obtain the default PKI location which may be different if the env variable was set
	defaultPKI, err := cmdhelper.GetZitiPKI()
	handleVariableError(err, cmdhelper.ZitiPKIVarName)

	cmd.PersistentFlags().BoolVarP(&options.Verbose, optionVerbose, "v", defaultVerbose, verboseDescription)
	cmd.PersistentFlags().StringVarP(&options.Output, optionOutput, "o", defaultOutput, outputDescription)
	cmd.PersistentFlags().StringVarP(&options.PKIPath, optionPKI, "", defaultPKI, pkiDescription)
}

func (data *ConfigTemplateValues) populateEnvVars() {

	// Get and add hostname to the params
	hostname, err := os.Hostname()
	handleVariableError(err, "hostname")

	// Get and add ziti home to the params
	zitiHome, err := cmdhelper.GetZitiHome()
	handleVariableError(err, cmdhelper.ZitiHomeVarName)

	// Get Ziti Controller Rawname
	zitiCtrlRawname, err := cmdhelper.GetZitiCtrlRawname()
	handleVariableError(err, cmdhelper.ZitiCtrlRawnameVarName)

	// Get Ziti Fabric Management Port
	zitiFabMgmtPort, err := cmdhelper.GetZitiFabMgmtPort()
	handleVariableError(err, cmdhelper.ZitiFabMgmtPortVarName)

	// Get Ziti Edge Controller API
	zitiEdgeCtrlAPI, err := cmdhelper.GetZitiEdgeControllerAPI()
	handleVariableError(err, cmdhelper.ZitiEdgeCtrlAPIVarName)

	// Get Ziti Signing Intermediate Name
	zitiSigningIntermediateName, err := cmdhelper.GetZitiSigningIntermediateName()
	handleVariableError(err, cmdhelper.ZitiSigningIntermediateNameVarName)

	// Get Ziti Edge Controller Port
	zitiEdgeCtrlPort, err := cmdhelper.GetZitiEdgeCtrlPort()
	handleVariableError(err, cmdhelper.ZitiEdgeCtrlPortVarName)

	// Get Ziti Edge Intermediate Name
	zitiEdgeIntermediateName, err := cmdhelper.GetZitiEdgeCtrlIntermediateName()
	handleVariableError(err, cmdhelper.ZitiEdgeCtrlPortVarName)

	// Get Ziti Edge Controller Hostname
	zitiEdgeCtrlHostname, err := cmdhelper.GetZitiEdgeCtrlHostname()
	handleVariableError(err, cmdhelper.ZitiEdgeCtrlHostnameVarName)

	// Get Ziti Controller Hostname
	zitiCtrlHostname, err := cmdhelper.GetZitiCtrlHostname()
	handleVariableError(err, cmdhelper.ZitiCtrlHostnameVarName)

	// Get Ziti fabric ctrl port
	zitiFabCtrlPort, err := cmdhelper.GetZitiFabCtrlPort()
	handleVariableError(err, cmdhelper.ZitiFabCtrlPortVarName)

	// Get Ziti PKI path
	zitiPKI, err := cmdhelper.GetZitiPKI()
	handleVariableError(err, cmdhelper.ZitiPKIVarName)

	// Get Ziti Edge Router Hostname
	zitiEdgeRouterHostName, err := cmdhelper.GetZitiEdgeRouterHostname()
	handleVariableError(err, cmdhelper.ZitiEdgeRouterHostnameVarName)

	// Get Ziti Edge Router Port
	zitiEdgeRouterPort, err := cmdhelper.GetZitiEdgeRouterPort()
	handleVariableError(err, cmdhelper.ZitiEdgeRouterPortVarName)

	data.ZitiPKI = zitiPKI
	data.ZitiHome = zitiHome
	data.Hostname = hostname
	data.ZitiFabMgmtPort = zitiFabMgmtPort
	data.ZitiSigningIntermediateName = zitiSigningIntermediateName
	data.ZitiCtrlRawname = zitiCtrlRawname
	data.ZitiEdgeCtrlIntermediateName = zitiEdgeIntermediateName
	data.ZitiEdgeCtrlHostname = zitiEdgeCtrlHostname
	data.Controller.EdgeCtrlAPI = zitiEdgeCtrlAPI
	data.Controller.EdgeCtrlPort = zitiEdgeCtrlPort
	data.Controller.Hostname = zitiCtrlHostname
	data.Controller.FabCtrlPort = zitiFabCtrlPort
	data.Router.Edge.Hostname = zitiEdgeRouterHostName
	data.Router.Edge.Port = zitiEdgeRouterPort
}

func (data *ConfigTemplateValues) populateDefaults() {
	data.Router.Listener.BindPort = constants.DefaultListenerBindPort
	data.Router.Listener.OutQueueSize = constants.DefaultOutQueueSize
	data.Router.Listener.ConnectTimeoutMs = constants.DefaultConnectTimeoutMs
	data.Router.Listener.GetSessionTimeoutS = constants.DefaultGetSessionTimeoutS
	data.Controller.EdgeAPISessionTimeoutMinutes = constants.DefaultEdgeAPISessionTimeoutMinutes
	data.Controller.WebListener.IdleTimeoutMS = constants.DefaultWebListenerIdleTimeoutMs
	data.Controller.WebListener.ReadTimeoutMS = constants.DefaultWebListenerReadTimeoutMs
	data.Controller.WebListener.WriteTimeoutMS = constants.DefaultWebListenerWriteTimeoutMs
	data.Controller.WebListener.MinTLSVersion = constants.DefaultWebListenerMinTLSVersion
	data.Controller.WebListener.MaxTLSVersion = constants.DefaultWebListenerMaxTLSVersion
	data.Router.Wss.WriteTimeout = constants.DefaultWSSWriteTimeout
	data.Router.Wss.ReadTimeout = constants.DefaultWSSReadTimeout
	data.Router.Wss.IdleTimeout = constants.DefaultWSSIdleTimeout
	data.Router.Wss.PongTimeout = constants.DefaultWSSPongTimeout
	data.Router.Wss.PingInterval = constants.DefaultWSSPingInterval
	data.Router.Wss.HandshakeTimeout = constants.DefaultWSSHandshakeTimeout
	data.Router.Wss.ReadBufferSize = constants.DefaultWSSReadBufferSize
	data.Router.Wss.WriteBufferSize = constants.DefaultWSSWriteBufferSize
	data.Router.Forwarder.LatencyProbeInterval = constants.DefaultLatencyProbeInterval
	data.Router.Forwarder.XgressDialQueueLength = constants.DefaultXgressDialQueueLength
	data.Router.Forwarder.XgressDialWorkerCount = constants.DefaultXgressDialWorkerCount
	data.Router.Forwarder.LinkDialQueueLength = constants.DefaultLinkDialQueueLength
	data.Router.Forwarder.LinkDialWorkerCount = constants.DefaultLinkDialWorkerCount
}

func handleVariableError(err error, varName string) {
	if err != nil {
		logrus.Errorf("Unable to get %s", varName)
	}
}
