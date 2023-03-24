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
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdHelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/constants"
	"os"
	"time"

	"github.com/openziti/channel/v2"
	edge "github.com/openziti/edge/controller/config"
	fabCtrl "github.com/openziti/fabric/controller"
	fabForwarder "github.com/openziti/fabric/router/forwarder"
	foundation "github.com/openziti/transport/v2"
	fabXweb "github.com/openziti/xweb/v2"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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

type CtrlValues struct {
	MinQueuedConnects          int
	MaxQueuedConnects          int
	DefaultQueuedConnects      int
	MinOutstandingConnects     int
	MaxOutstandingConnects     int
	DefaultOutstandingConnects int
	MinConnectTimeout          time.Duration
	MaxConnectTimeout          time.Duration
	DefaultConnectTimeout      time.Duration
	ListenerAddress            string
	ListenerPort               string
}

type HealthChecksValues struct {
	Interval     time.Duration
	Timeout      time.Duration
	InitialDelay time.Duration
}

type EdgeApiValues struct {
	APIActivityUpdateBatchSize int
	APIActivityUpdateInterval  time.Duration
	SessionTimeout             time.Duration
	Address                    string
	Port                       string
}

type EdgeEnrollmentValues struct {
	SigningCert                 string
	SigningCertKey              string
	EdgeIdentityDuration        time.Duration
	EdgeRouterDuration          time.Duration
	DefaultEdgeIdentityDuration time.Duration
	DefaultEdgeRouterDuration   time.Duration
}

type WebValues struct {
	BindPoints BindPointsValues
	Identity   IdentityValues
	Options    WebOptionsValues
}

type BindPointsValues struct {
	InterfaceAddress string
	InterfacePort    string
	AddressAddress   string
	AddressPort      string
}

type IdentityValues struct {
	Ca         string
	Key        string
	ServerCert string
	Cert       string
}

type WebOptionsValues struct {
	IdleTimeout   time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	MinTLSVersion string
	MaxTLSVersion string
}

type ControllerTemplateValues struct {
	Identity       IdentityValues
	Ctrl           CtrlValues
	HealthChecks   HealthChecksValues
	EdgeApi        EdgeApiValues
	EdgeEnrollment EdgeEnrollmentValues
	Web            WebValues
}

type RouterTemplateValues struct {
	Name               string
	IsPrivate          bool
	IsFabric           bool
	IsWss              bool
	TunnelerMode       string
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
	Hostname         string
	Port             string
	IPOverride       string
	AdvertisedHost   string
	LanInterface     string
	ListenerBindPort string
}

type WSSRouterTemplateValues struct {
	WriteTimeout      time.Duration
	ReadTimeout       time.Duration
	IdleTimeout       time.Duration
	PongTimeout       time.Duration
	PingInterval      time.Duration
	HandshakeTimeout  time.Duration
	ReadBufferSize    int
	WriteBufferSize   int
	EnableCompression bool
}

type RouterForwarderTemplateValues struct {
	LatencyProbeInterval  time.Duration
	XgressDialQueueLength int
	XgressDialWorkerCount int
	LinkDialQueueLength   int
	LinkDialWorkerCount   int
}

type RouterListenerTemplateValues struct {
	ConnectTimeout    time.Duration
	GetSessionTimeout time.Duration
	OutQueueSize      int
}

var workingDir string
var data = &ConfigTemplateValues{}

func init() {
	workingDir, _ = cmdHelper.GetZitiHome()
}

// NewCmdCreateConfig creates a command object for the "config" command
func NewCmdCreateConfig() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Creates a config file for specified Ziti component using environment variables",
		Aliases: []string{"cfg"},
		Run: func(cmd *cobra.Command, args []string) {
			cmdHelper.CheckErr(cmd.Help())
		},
	}

	cmd.AddCommand(NewCmdCreateConfigController())
	cmd.AddCommand(NewCmdCreateConfigRouter())
	cmd.AddCommand(NewCmdCreateConfigEnvironment())

	return cmd
}

// Add flags that are global to all "create config" commands
func (options *CreateConfigOptions) addCreateFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVarP(&options.Verbose, optionVerbose, "v", defaultVerbose, verboseDescription)
	cmd.PersistentFlags().StringVarP(&options.Output, optionOutput, "o", defaultOutput, outputDescription)
}

func (data *ConfigTemplateValues) populateConfigValues() {

	// Get and add hostname to the params
	hostname, err := os.Hostname()
	handleVariableError(err, "hostname")

	// Get and add ziti home to the params
	zitiHome, err := cmdHelper.GetZitiHome()
	handleVariableError(err, constants.ZitiHomeVarName)

	// Get Ziti Controller ctrl:listener address and port
	ctrlListenerAddress, err := cmdHelper.GetCtrlListenerAddress()
	handleVariableError(err, constants.CtrlListenerAddressVarName)
	ctrlListenerPort, err := cmdHelper.GetCtrlListenerPort()
	handleVariableError(err, constants.CtrlListenerPortVarName)

	// Get Ziti Controller edge:api address and port
	ctrlEdgeApiAddress, err := cmdHelper.GetCtrlEdgeApiAddress()
	handleVariableError(err, constants.CtrlEdgeApiAddressVarName)
	ctrlEdgeApiPort, err := cmdHelper.GetCtrlEdgeApiPort()
	handleVariableError(err, constants.CtrlEdgeApiPortVarName)

	// Get Ziti Controller Identity edge:enrollment duration
	ctrlEdgeIdentityEnrollmentDuration, err := cmdHelper.GetCtrlEdgeIdentityEnrollmentDuration()
	handleVariableError(err, constants.CtrlEdgeIdentityEnrollmentDurationVarName)

	// Get Ziti Controller Router edge:enrollment enrollment duration
	ctrlEdgeRouterEnrollmentDuration, err := cmdHelper.GetCtrlEdgeRouterEnrollmentDuration()
	handleVariableError(err, constants.CtrlEdgeRouterEnrollmentDurationVarName)

	// Get Ziti Controller web:bindPoints interface address and port
	ctrlEdgeInterfaceAddress, err := cmdHelper.GetCtrlEdgeInterfaceAddress()
	handleVariableError(err, constants.CtrlEdgeInterfaceAddressVarName)
	ctrlEdgeInterfacePort, err := cmdHelper.GetCtrlEdgeInterfacePort()
	handleVariableError(err, constants.CtrlEdgeInterfacePortVarName)

	// Get Ziti Controller web:bindPoints address address and port
	ctrlEdgeAdvertisedAddress, err := cmdHelper.GetCtrlEdgeAdvertisedAddress()
	handleVariableError(err, constants.CtrlEdgeAdvertisedAddressVarName)
	ctrlEdgeAdvertisedPort, err := cmdHelper.GetCtrlEdgeAdvertisedPort()
	handleVariableError(err, constants.CtrlEdgeAdvertisedPortVarName)

	// Get Ziti Edge Router Port
	zitiEdgeRouterPort, err := cmdHelper.GetZitiEdgeRouterPort()
	handleVariableError(err, constants.ZitiEdgeRouterPortVarName)

	zitiEdgeRouterListenerBindPort, err := cmdHelper.GetZitiEdgeRouterListenerBindPort()
	handleVariableError(err, constants.ZitiEdgeRouterListenerBindPortVarName)

	data.ZitiHome = zitiHome
	data.Hostname = hostname
	// ************* Controller Values ************
	// Identities are handled in create_config_controller
	// ctrl:
	data.Controller.Ctrl.MinQueuedConnects = channel.MinQueuedConnects
	data.Controller.Ctrl.MaxQueuedConnects = channel.MaxQueuedConnects
	data.Controller.Ctrl.DefaultQueuedConnects = channel.DefaultQueuedConnects
	data.Controller.Ctrl.MinOutstandingConnects = channel.MinOutstandingConnects
	data.Controller.Ctrl.MaxOutstandingConnects = channel.MaxOutstandingConnects
	data.Controller.Ctrl.DefaultOutstandingConnects = channel.DefaultOutstandingConnects
	data.Controller.Ctrl.MinConnectTimeout = channel.MinConnectTimeout
	data.Controller.Ctrl.MaxConnectTimeout = channel.MaxConnectTimeout
	data.Controller.Ctrl.DefaultConnectTimeout = channel.DefaultConnectTimeout
	data.Controller.Ctrl.ListenerAddress = ctrlListenerAddress
	data.Controller.Ctrl.ListenerPort = ctrlListenerPort
	// healthChecks:
	data.Controller.HealthChecks.Interval = fabCtrl.DefaultHealthChecksBoltCheckInterval
	data.Controller.HealthChecks.Timeout = fabCtrl.DefaultHealthChecksBoltCheckTimeout
	data.Controller.HealthChecks.InitialDelay = fabCtrl.DefaultHealthChecksBoltCheckInitialDelay
	// edge:
	data.Controller.EdgeApi.APIActivityUpdateBatchSize = edge.DefaultEdgeApiActivityUpdateBatchSize
	data.Controller.EdgeApi.APIActivityUpdateInterval = edge.DefaultEdgeAPIActivityUpdateInterval
	data.Controller.EdgeApi.SessionTimeout = edge.DefaultEdgeSessionTimeout
	data.Controller.EdgeApi.Address = ctrlEdgeApiAddress
	data.Controller.EdgeApi.Port = ctrlEdgeApiPort
	data.Controller.EdgeEnrollment.EdgeIdentityDuration = ctrlEdgeIdentityEnrollmentDuration
	data.Controller.EdgeEnrollment.EdgeRouterDuration = ctrlEdgeRouterEnrollmentDuration
	data.Controller.EdgeEnrollment.DefaultEdgeIdentityDuration = edge.DefaultEdgeEnrollmentDuration
	data.Controller.EdgeEnrollment.DefaultEdgeRouterDuration = edge.DefaultEdgeEnrollmentDuration
	// web:
	data.Controller.Web.BindPoints.InterfaceAddress = ctrlEdgeInterfaceAddress
	data.Controller.Web.BindPoints.InterfacePort = ctrlEdgeInterfacePort
	data.Controller.Web.BindPoints.AddressAddress = ctrlEdgeAdvertisedAddress
	data.Controller.Web.BindPoints.AddressPort = ctrlEdgeAdvertisedPort
	// Web Identities are handled in create_config_controller
	data.Controller.Web.Options.IdleTimeout = edge.DefaultHttpIdleTimeout
	data.Controller.Web.Options.ReadTimeout = edge.DefaultHttpReadTimeout
	data.Controller.Web.Options.WriteTimeout = edge.DefaultHttpWriteTimeout
	data.Controller.Web.Options.MinTLSVersion = fabXweb.ReverseTlsVersionMap[fabXweb.MinTLSVersion]
	data.Controller.Web.Options.MaxTLSVersion = fabXweb.ReverseTlsVersionMap[fabXweb.MaxTLSVersion]

	// ************* Router Values ************
	data.Router.Edge.Port = zitiEdgeRouterPort
	data.Router.Edge.ListenerBindPort = zitiEdgeRouterListenerBindPort
	data.Router.Listener.GetSessionTimeout = constants.DefaultGetSessionTimeout

	data.Router.Wss.WriteTimeout = foundation.DefaultWsWriteTimeout
	data.Router.Wss.ReadTimeout = foundation.DefaultWsReadTimeout
	data.Router.Wss.IdleTimeout = foundation.DefaultWsIdleTimeout
	data.Router.Wss.PongTimeout = foundation.DefaultWsPongTimeout
	data.Router.Wss.PingInterval = foundation.DefaultWsPingInterval
	data.Router.Wss.HandshakeTimeout = foundation.DefaultWsHandshakeTimeout
	data.Router.Wss.ReadBufferSize = foundation.DefaultWsReadBufferSize
	data.Router.Wss.WriteBufferSize = foundation.DefaultWsWriteBufferSize
	data.Router.Wss.EnableCompression = foundation.DefaultWsEnableCompression
	data.Router.Forwarder.LatencyProbeInterval = fabForwarder.DefaultLatencyProbeInterval
	data.Router.Forwarder.XgressDialQueueLength = fabForwarder.DefaultXgressDialWorkerQueueLength
	data.Router.Forwarder.XgressDialWorkerCount = fabForwarder.DefaultXgressDialWorkerCount
	data.Router.Forwarder.LinkDialQueueLength = fabForwarder.DefaultLinkDialQueueLength
	data.Router.Forwarder.LinkDialWorkerCount = fabForwarder.DefaultLinkDialWorkerCount
	data.Router.Listener.OutQueueSize = channel.DefaultOutQueueSize
	data.Router.Listener.ConnectTimeout = channel.DefaultConnectTimeout
}

func handleVariableError(err error, varName string) {
	if err != nil {
		logrus.Errorf("Unable to get %s: %v", varName, err)
	}
}
