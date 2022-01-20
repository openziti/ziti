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
	ZitiPKI                  string
	ZitiCtrlIntermediateName string
	ZitiCtrlHostname         string
	ZitiFabCtrlPort          string

	// Controller specific
	CtrlListener                 string
	MgmtListener                 string
	ZitiHome                     string
	Hostname                     string
	ZitiFabMgmtPort              string
	ZitiEdgeCtrlAPI              string
	ZitiSigningIntermediateName  string
	ZitiEdgeCtrlPort             string
	ZitiCtrlRawname              string
	ZitiEdgeCtrlIntermediateName string
	ZitiEdgeCtrlHostname         string

	// Router specific
	RouterName             string
	ZitiEdgeRouterHostname string
	ZitiEdgeRouterPort     string
	ZitiEdgeWSSRouterName  string
	WssEnabled             bool
	IsPrivate              bool
	IsFabricRouter         bool

	// Default values
	ListenerBindPort             int
	OutQueueSize                 int
	EdgeAPISessionTimeoutMinutes int
	WebListenerIdleTimeoutMS     int
	WebListenerReadTimeoutMS     int
	WebListenerWriteTimeoutMS    int
	WebListenerMinTLSVersion     string
	WebListenerMaxTLSVersion     string
	WssWriteTimeout              int
	WssReadTimeout               int
	WssIdleTimeout               int
	WssPongTimeout               int
	WssPingInterval              int
	WssHandshakeTimeout          int
	WssReadBufferSize            int
	WssWriteBufferSize           int
	LatencyProbeInterval         int
	XgressDialQueueLength        int
	XgressDialWorkerCount        int
	LinkDialQueueLength          int
	LinkDialWorkerCount          int
	ConnectTimeoutMs             int
	GetSessionTimeoutS           int
}

type ControllerConfigValues struct {
	CtrlListener                 string
	MgmtListener                 string
	ZitiHome                     string
	Hostname                     string
	ZitiFabMgmtPort              string
	ZitiEdgeCtrlAPI              string
	ZitiSigningIntermediateName  string
	ZitiEdgeCtrlPort             string
	ZitiCtrlRawname              string
	ZitiEdgeCtrlIntermediateName string
	ZitiEdgeCtrlHostname         string
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

	// Get Ziti Controller Intermediate Name
	zitiCtrlIntName, err := cmdhelper.GetZitiCtrlIntermediateName()
	handleVariableError(err, cmdhelper.ZitiCtrlIntermediateNameVarName)

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
	data.ZitiCtrlIntermediateName = zitiCtrlIntName
	data.ZitiCtrlHostname = zitiCtrlHostname
	data.ZitiFabCtrlPort = zitiFabCtrlPort
	data.ZitiHome = zitiHome
	data.Hostname = hostname
	data.ZitiFabMgmtPort = zitiFabMgmtPort
	data.ZitiEdgeCtrlAPI = zitiEdgeCtrlAPI
	data.ZitiSigningIntermediateName = zitiSigningIntermediateName
	data.ZitiEdgeCtrlPort = zitiEdgeCtrlPort
	data.ZitiCtrlRawname = zitiCtrlRawname
	data.ZitiEdgeCtrlIntermediateName = zitiEdgeIntermediateName
	data.ZitiEdgeCtrlHostname = zitiEdgeCtrlHostname
	data.ZitiEdgeRouterHostname = zitiEdgeRouterHostName
	data.ZitiEdgeRouterPort = zitiEdgeRouterPort
}

func (data *ConfigTemplateValues) populateDefaults() {
	data.ListenerBindPort = constants.DefaultListenerBindPort
	data.OutQueueSize = constants.DefaultOutQueueSize
	data.EdgeAPISessionTimeoutMinutes = constants.DefaultEdgeAPISessionTimeoutMinutes
	data.WebListenerIdleTimeoutMS = constants.DefaultWebListenerIdleTimeoutMs
	data.WebListenerReadTimeoutMS = constants.DefaultWebListenerReadTimeoutMs
	data.WebListenerWriteTimeoutMS = constants.DefaultWebListenerWriteTimeoutMs
	data.WebListenerMinTLSVersion = constants.DefaultWebListenerMinTLSVersion
	data.WebListenerMaxTLSVersion = constants.DefaultWebListenerMaxTLSVersion
	data.WssWriteTimeout = constants.DefaultWSSWriteTimeout
	data.WssReadTimeout = constants.DefaultWSSReadTimeout
	data.WssIdleTimeout = constants.DefaultWSSIdleTimeout
	data.WssPongTimeout = constants.DefaultWSSPongTimeout
	data.WssPingInterval = constants.DefaultWSSPingInterval
	data.WssHandshakeTimeout = constants.DefaultWSSHandshakeTimeout
	data.WssReadBufferSize = constants.DefaultWSSReadBufferSize
	data.WssWriteBufferSize = constants.DefaultWSSWriteBufferSize
	data.LatencyProbeInterval = constants.DefaultLatencyProbeInterval
	data.XgressDialQueueLength = constants.DefaultXgressDialQueueLength
	data.XgressDialWorkerCount = constants.DefaultXgressDialWorkerCount
	data.LinkDialQueueLength = constants.DefaultLinkDialQueueLength
	data.LinkDialWorkerCount = constants.DefaultLinkDialWorkerCount
	data.ConnectTimeoutMs = constants.DefaultConnectTimeoutMs
	data.GetSessionTimeoutS = constants.DefaultGetSessionTimeoutS
}

func handleVariableError(err error, varName string) {
	if err != nil {
		logrus.Errorf("Unable to get %s", varName)
	}
}
