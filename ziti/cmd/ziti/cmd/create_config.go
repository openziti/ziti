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
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ziti/constants"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
)

const (
	optionVerbose      = "verbose"
	defaultVerbose     = false
	verboseDescription = "Enable verbose logging. Logging will be sent to stdout if the config output is sent to a file. If output is sent to stdout, logging will be sent to stderr"
	optionOutput       = "output"
	defaultOutput      = "stdout"
	outputDescription  = "designated output destination for config, use \"stdout\" or a filepath."
)

// CreateConfigOptions the options for the create config command
type CreateConfigOptions struct {
	common.CommonOptions

	Output       string
	DatabaseFile string
}

type ConfigTemplateValues struct {
	ZitiHome string
	Hostname string

	Controller ControllerTemplateValues
	Router     RouterTemplateValues
}

type ControllerTemplateValues struct {
	Name                 string
	Port                 string
	AdvertisedAddress    string
	ListenerAddress      string
	MgmtListenerHostPort string
	IdentityCert         string
	IdentityServerCert   string
	IdentityKey          string
	IdentityCA           string
	Edge                 EdgeControllerValues
	WebListener          ControllerWebListenerValues
	HealthCheck          ControllerHealthCheckValues
}

type EdgeControllerValues struct {
	ZitiSigningCert string
	ZitiSigningKey  string

	APISessionTimeoutMinutes int
	ListenerHostPort         string
	AdvertisedHostPort       string
	IdentityCert             string
	IdentityServerCert       string
	IdentityKey              string
	IdentityCA               string
}

type ControllerWebListenerValues struct {
	IdleTimeoutMS  int
	ReadTimeoutMS  int
	WriteTimeoutMS int
	MinTLSVersion  string
	MaxTLSVersion  string
}

type ControllerHealthCheckValues struct {
	IntervalSec     int
	TimeoutSec      int
	InitialDelaySec int
}

type RouterTemplateValues struct {
	Name               string
	IsPrivate          bool
	IsFabric           bool
	IsWss              bool
	IdentityCert       string
	IdentityServerCert string
	IdentityKey        string
	IdentityCA         string
	Edge               EdgeRouterTemplateValues
	Wss                WSSRouterTemplateValues
	Forwarder          RouterForwarderTemplateValues
	Listener           RouterListenerTemplateValues
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

var workingDir string

func init() {
	zh := os.Getenv("ZITI_HOME")
	if zh == "" {
		wd, err := os.Getwd()
		if wd == "" || err != nil {
			//on error just use "."
			workingDir = "."
		}
	}

	workingDir = cmdhelper.NormalizePath(zh)
}

// NewCmdCreateConfig creates a command object for the "config" command
func NewCmdCreateConfig() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Creates a config file for specified Ziti component using environment variables",
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
	cmd.AddCommand(NewCmdCreateConfigRouter(templateData))
	cmd.AddCommand(NewCmdCreateConfigEnvironment(templateData))

	return cmd
}

// Add flags that are global to all "create config" commands
func (options *CreateConfigOptions) addCreateFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVarP(&options.Verbose, optionVerbose, "v", defaultVerbose, verboseDescription)
	cmd.PersistentFlags().StringVarP(&options.Output, optionOutput, "o", defaultOutput, outputDescription)
}

func (data *ConfigTemplateValues) populateEnvVars() {

	// Get and add hostname to the params
	hostname, err := os.Hostname()
	handleVariableError(err, "hostname")

	// Get and add ziti home to the params
	zitiHome, err := cmdhelper.GetZitiHome()
	handleVariableError(err, constants.ZitiHomeVarName)

	// Get Ziti Controller Name
	zitiCtrlHostname, err := cmdhelper.GetZitiCtrlName()
	handleVariableError(err, constants.ZitiCtrlNameVarName)

	// Get Ziti Edge Router Port
	zitiEdgeRouterPort, err := cmdhelper.GetZitiEdgeRouterPort()
	handleVariableError(err, constants.ZitiEdgeRouterPortVarName)

	// Get Ziti Controller Listener Address
	zitiCtrlListenerAddress, err := cmdhelper.GetZitiCtrlListenerAddress()
	handleVariableError(err, constants.ZitiCtrlListenerAddressVarName)

	// Get Ziti Controller Advertised Address
	zitiCtrlAdvertisedAddress, err := cmdhelper.GetZitiCtrlAdvertisedAddress()
	handleVariableError(err, constants.ZitiCtrlAdvertisedAddressVarName)

	// Get Ziti Controller Port
	zitiCtrlPort, err := cmdhelper.GetZitiCtrlPort()
	handleVariableError(err, constants.ZitiCtrlPortVarName)

	// Get Ziti Controller Management Host and Port
	zitiCtrlMgmtListenerHostPort, err := cmdhelper.GetZitiCtrlMgmtListenerHostPort()
	handleVariableError(err, constants.ZitiCtrlMgmtListenerHostPortVarName)

	// Get Ziti Edge Controller Listener Host and Port
	zitiEdgeCtrlListenerHostPort, err := cmdhelper.GetZitiEdgeCtrlListenerHostPort()
	handleVariableError(err, constants.ZitiEdgeCtrlListenerHostPortVarName)

	// Get Ziti Edge Controller Advertised Host and Port
	zitiEdgeCtrlAdvertisedHostPort, err := cmdhelper.GetZitiEdgeCtrlAdvertisedHostPort()
	handleVariableError(err, constants.ZitiEdgeCtrlAdvertisedHostPortVarName)

	data.ZitiHome = zitiHome
	data.Hostname = hostname
	data.Controller.Name = zitiCtrlHostname
	data.Controller.ListenerAddress = zitiCtrlListenerAddress
	data.Controller.AdvertisedAddress = zitiCtrlAdvertisedAddress
	data.Controller.Port = zitiCtrlPort
	data.Controller.MgmtListenerHostPort = zitiCtrlMgmtListenerHostPort
	data.Controller.Edge.ListenerHostPort = zitiEdgeCtrlListenerHostPort
	data.Controller.Edge.AdvertisedHostPort = zitiEdgeCtrlAdvertisedHostPort
	data.Router.Edge.Port = zitiEdgeRouterPort
}

func (data *ConfigTemplateValues) populateDefaults() {
	data.Router.Listener.BindPort = constants.DefaultListenerBindPort
	data.Router.Listener.OutQueueSize = constants.DefaultOutQueueSize
	data.Router.Listener.ConnectTimeoutMs = constants.DefaultConnectTimeoutMs
	data.Router.Listener.GetSessionTimeoutS = constants.DefaultGetSessionTimeoutS
	data.Controller.Edge.APISessionTimeoutMinutes = constants.DefaultEdgeAPISessionTimeoutMinutes
	data.Controller.WebListener.IdleTimeoutMS = constants.DefaultWebListenerIdleTimeoutMs
	data.Controller.WebListener.ReadTimeoutMS = constants.DefaultWebListenerReadTimeoutMs
	data.Controller.WebListener.WriteTimeoutMS = constants.DefaultWebListenerWriteTimeoutMs
	data.Controller.WebListener.MinTLSVersion = constants.DefaultWebListenerMinTLSVersion
	data.Controller.WebListener.MaxTLSVersion = constants.DefaultWebListenerMaxTLSVersion
	data.Controller.HealthCheck.TimeoutSec = constants.DefaultControllerHealthCheckTimeoutSec
	data.Controller.HealthCheck.IntervalSec = constants.DefaultControllerHealthCheckIntervalSec
	data.Controller.HealthCheck.InitialDelaySec = constants.DefaultControllerHealthCheckDelaySec
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
		logrus.Errorf("Unable to get %s: %v", varName, err)
	}
}
