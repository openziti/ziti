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

type ControllerTemplateValues struct {
	Name                        string
	Port                        string
	AdvertisedAddress           string
	ListenerAddress             string
	IdentityCert                string
	IdentityServerCert          string
	IdentityKey                 string
	IdentityCA                  string
	MinQueuedConnects           int
	MaxQueuedConnects           int
	DefaultQueuedConnects       int
	MinOutstandingConnects      int
	MaxOutstandingConnects      int
	DefaultOutstandingConnects  int
	MinConnectTimeout           time.Duration
	MaxConnectTimeout           time.Duration
	DefaultConnectTimeout       time.Duration
	EdgeIdentityDuration        time.Duration
	EdgeRouterDuration          time.Duration
	DefaultEdgeIdentityDuration time.Duration
	DefaultEdgeRouterDuration   time.Duration
	Edge                        EdgeControllerValues
	WebListener                 ControllerWebListenerValues
	HealthCheck                 ControllerHealthCheckValues
}

type EdgeControllerValues struct {
	AdvertisedPort  string
	ZitiSigningCert string
	ZitiSigningKey  string

	APIActivityUpdateBatchSize int
	APIActivityUpdateInterval  time.Duration
	APISessionTimeout          time.Duration
	ListenerHostPort           string
	AdvertisedHostPort         string
	IdentityCert               string
	IdentityServerCert         string
	IdentityKey                string
	IdentityCA                 string
}

type ControllerWebListenerValues struct {
	IdleTimeout   time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	MinTLSVersion string
	MaxTLSVersion string
}

type ControllerHealthCheckValues struct {
	Interval     time.Duration
	Timeout      time.Duration
	InitialDelay time.Duration
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
	zh := os.Getenv("ZITI_HOME")
	if zh == "" {
		wd, err := os.Getwd()
		if wd == "" || err != nil {
			//on error just use "."
			workingDir = "."
		}
	}

	workingDir = cmdHelper.NormalizePath(zh)
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

func (data *ConfigTemplateValues) populateEnvVars() {

	// Get and add hostname to the params
	hostname, err := os.Hostname()
	handleVariableError(err, "hostname")

	// Get and add ziti home to the params
	zitiHome, err := cmdHelper.GetZitiHome()
	handleVariableError(err, constants.ZitiHomeVarName)

	// Get Ziti Controller Name
	zitiCtrlHostname, err := cmdHelper.GetZitiCtrlName()
	handleVariableError(err, constants.ZitiCtrlNameVarName)

	// Get Ziti Edge Router Port
	zitiEdgeRouterPort, err := cmdHelper.GetZitiEdgeRouterPort()
	handleVariableError(err, constants.ZitiEdgeRouterPortVarName)

	// Get Ziti Controller Listener Address
	zitiCtrlListenerAddress, err := cmdHelper.GetZitiCtrlListenerAddress()
	handleVariableError(err, constants.ZitiCtrlListenerAddressVarName)

	// Get Ziti Controller Advertised Address
	zitiCtrlAdvertisedAddress, err := cmdHelper.GetZitiCtrlAdvertisedAddress()
	handleVariableError(err, constants.ZitiCtrlAdvertisedAddressVarName)

	// Get Ziti Controller Port
	zitiCtrlPort, err := cmdHelper.GetZitiCtrlPort()
	handleVariableError(err, constants.ZitiCtrlPortVarName)

	// Get Ziti Edge Controller Listener Host and Port
	zitiEdgeCtrlListenerHostPort, err := cmdHelper.GetZitiEdgeCtrlListenerHostPort()
	handleVariableError(err, constants.ZitiEdgeCtrlListenerHostPortVarName)

	// Get Ziti Edge Controller Advertised Host and Port
	zitiEdgeCtrlAdvertisedHostPort, err := cmdHelper.GetZitiEdgeCtrlAdvertisedHostPort()
	handleVariableError(err, constants.ZitiEdgeCtrlAdvertisedHostPortVarName)

	// Get Ziti Edge Controller Advertised Port
	zitiEdgeCtrlAdvertisedPort, err := cmdHelper.GetZitiEdgeCtrlAdvertisedPort()
	handleVariableError(err, constants.ZitiEdgeCtrlAdvertisedPortVarName)

	// Get Ziti edge Identity enrollment duration
	zitiEdgeIdentityEnrollmentDuration, err := cmdHelper.GetZitiEdgeIdentityEnrollmentDuration()
	handleVariableError(err, constants.ZitiEdgeIdentityEnrollmentDurationVarName)

	// Get Ziti edge Router enrollment duration
	zitiEdgeRouterEnrollmentDuration, err := cmdHelper.GetZitiEdgeRouterEnrollmentDuration()
	handleVariableError(err, constants.ZitiEdgeRouterEnrollmentDurationVarName)

	zitiEdgeRouterListenerBindPort, err := cmdHelper.GetZitiEdgeRouterListenerBindPort()
	handleVariableError(err, constants.ZitiEdgeRouterListenerBindPortVarName)

	data.ZitiHome = zitiHome
	data.Hostname = hostname
	data.Controller.Name = zitiCtrlHostname
	data.Controller.ListenerAddress = zitiCtrlListenerAddress
	data.Controller.AdvertisedAddress = zitiCtrlAdvertisedAddress
	data.Controller.Port = zitiCtrlPort
	data.Controller.Edge.ListenerHostPort = zitiEdgeCtrlListenerHostPort
	data.Controller.Edge.AdvertisedHostPort = zitiEdgeCtrlAdvertisedHostPort
	data.Router.Edge.Port = zitiEdgeRouterPort
	data.Router.Edge.ListenerBindPort = zitiEdgeRouterListenerBindPort
	data.Controller.Edge.AdvertisedPort = zitiEdgeCtrlAdvertisedPort
	data.Controller.EdgeIdentityDuration = zitiEdgeIdentityEnrollmentDuration
	data.Controller.EdgeRouterDuration = zitiEdgeRouterEnrollmentDuration
	data.Controller.DefaultEdgeIdentityDuration = edge.DefaultEdgeEnrollmentDuration
	data.Controller.DefaultEdgeRouterDuration = edge.DefaultEdgeEnrollmentDuration
}

func (data *ConfigTemplateValues) populateDefaults() {
	data.Router.Listener.GetSessionTimeout = constants.DefaultGetSessionTimeout

	data.Controller.MinQueuedConnects = channel.MinQueuedConnects
	data.Controller.MaxQueuedConnects = channel.MaxQueuedConnects
	data.Controller.DefaultQueuedConnects = channel.DefaultQueuedConnects
	data.Controller.MinOutstandingConnects = channel.MinOutstandingConnects
	data.Controller.MaxOutstandingConnects = channel.MaxOutstandingConnects
	data.Controller.DefaultOutstandingConnects = channel.DefaultOutstandingConnects
	data.Controller.MinConnectTimeout = channel.MinConnectTimeout
	data.Controller.MaxConnectTimeout = channel.MaxConnectTimeout
	data.Controller.DefaultConnectTimeout = channel.DefaultConnectTimeout
	data.Controller.HealthCheck.Timeout = fabCtrl.DefaultHealthChecksBoltCheckTimeout
	data.Controller.HealthCheck.Interval = fabCtrl.DefaultHealthChecksBoltCheckInterval
	data.Controller.HealthCheck.InitialDelay = fabCtrl.DefaultHealthChecksBoltCheckInitialDelay
	data.Controller.Edge.APIActivityUpdateBatchSize = edge.DefaultEdgeApiActivityUpdateBatchSize
	data.Controller.Edge.APIActivityUpdateInterval = edge.DefaultEdgeAPIActivityUpdateInterval
	data.Controller.Edge.APISessionTimeout = edge.DefaultEdgeSessionTimeout
	data.Controller.WebListener.IdleTimeout = edge.DefaultHttpIdleTimeout
	data.Controller.WebListener.ReadTimeout = edge.DefaultHttpReadTimeout
	data.Controller.WebListener.WriteTimeout = edge.DefaultHttpWriteTimeout
	data.Controller.WebListener.MinTLSVersion = fabXweb.ReverseTlsVersionMap[fabXweb.MinTLSVersion]
	data.Controller.WebListener.MaxTLSVersion = fabXweb.ReverseTlsVersionMap[fabXweb.MaxTLSVersion]
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
