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

package create

import (
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdHelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/constants"
	"regexp"
	"time"

	"github.com/openziti/channel/v2"
	foundation "github.com/openziti/transport/v2"
	fabXweb "github.com/openziti/xweb/v2"
	edge "github.com/openziti/ziti/controller/config"
	fabForwarder "github.com/openziti/ziti/router/forwarder"
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
	ZitiHome              string
	HostnameOrNetworkName string

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
	AdvertisedAddress          string
	AdvertisedPort             string
	BindAddress                string
	AltAdvertisedAddress       string
	MinClusterSize             int
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
	Console          ConsoleValues
}

type ConsoleValues struct {
	Disabled      bool
	Location      string
}

type IdentityValues struct {
	Ca              string
	Key             string
	ServerCert      string
	Cert            string
	AltServerCert   string
	AltServerKey    string
	AltCertsEnabled bool
}

type DatabaseValues struct {
	DatabaseFile string
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
	Database       DatabaseValues
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
	AltServerCert      string
	AltServerKey       string
	AltCertsEnabled    bool
	Edge               EdgeRouterTemplateValues
	Wss                WSSRouterTemplateValues
	Forwarder          RouterForwarderTemplateValues
	Listener           RouterListenerTemplateValues
}

type EdgeRouterTemplateValues struct {
	Port             string
	IPOverride       string
	AdvertisedHost   string
	LanInterface     string
	Resolver         string
	DnsSvcIpRange    string
	ListenerBindPort string
	CsrC             string
	CsrST            string
	CsrL             string
	CsrO             string
	CsrOU            string
	CsrSans          string
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

	opts := &CreateConfigRouterOptions{}
	cmd.AddCommand(NewCmdCreateConfigController().Command)
	cmd.AddCommand(NewCmdCreateConfigRouter(opts).Command)
	cmd.AddCommand(NewCmdCreateConfigEnvironment())

	return cmd
}

// Add flags that are global to all "create config" commands
func (options *CreateConfigOptions) addCreateFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVarP(&options.Verbose, optionVerbose, "v", defaultVerbose, verboseDescription)
	cmd.PersistentFlags().StringVarP(&options.Output, optionOutput, "o", defaultOutput, outputDescription)
}

func (data *ConfigTemplateValues) PopulateConfigValues() {

	// Get and add hostname to the params
	data.HostnameOrNetworkName = cmdHelper.HostnameOrNetworkName()

	// Get and add ziti home to the params
	zitiHome := cmdHelper.GetZitiHome()

	data.ZitiHome = zitiHome
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
	data.Controller.Ctrl.AdvertisedAddress = cmdHelper.GetCtrlAdvertisedAddress()
	data.Controller.Ctrl.AltAdvertisedAddress = cmdHelper.GetCtrlEdgeAltAdvertisedAddress()
	data.Controller.Ctrl.BindAddress = cmdHelper.GetCtrlBindAddress()
	data.Controller.Ctrl.AdvertisedPort = cmdHelper.GetCtrlAdvertisedPort()
	data.Controller.Database.DatabaseFile = cmdHelper.GetCtrlDatabaseFile()
	// healthChecks:
	data.Controller.HealthChecks.Interval = edge.DefaultHealthChecksBoltCheckInterval
	data.Controller.HealthChecks.Timeout = edge.DefaultHealthChecksBoltCheckTimeout
	data.Controller.HealthChecks.InitialDelay = edge.DefaultHealthChecksBoltCheckInitialDelay
	// edge:
	data.Controller.EdgeApi.APIActivityUpdateBatchSize = edge.DefaultEdgeApiActivityUpdateBatchSize
	data.Controller.EdgeApi.APIActivityUpdateInterval = edge.DefaultEdgeAPIActivityUpdateInterval
	data.Controller.EdgeApi.SessionTimeout = edge.DefaultEdgeSessionTimeout
	data.Controller.EdgeApi.Address = cmdHelper.GetCtrlEdgeAltAdvertisedAddress()
	data.Controller.EdgeApi.Port = cmdHelper.GetCtrlEdgeAdvertisedPort()
	data.Controller.EdgeEnrollment.EdgeIdentityDuration = cmdHelper.GetCtrlEdgeIdentityEnrollmentDuration()
	data.Controller.EdgeEnrollment.EdgeRouterDuration = cmdHelper.GetCtrlEdgeRouterEnrollmentDuration()
	data.Controller.EdgeEnrollment.DefaultEdgeIdentityDuration = edge.DefaultEdgeEnrollmentDuration
	data.Controller.EdgeEnrollment.DefaultEdgeRouterDuration = edge.DefaultEdgeEnrollmentDuration
	// web:
	data.Controller.Web.BindPoints.InterfaceAddress = cmdHelper.GetCtrlEdgeBindAddress()
	data.Controller.Web.BindPoints.InterfacePort = cmdHelper.GetCtrlEdgeAdvertisedPort()
	data.Controller.Web.BindPoints.AddressAddress = cmdHelper.GetCtrlEdgeAltAdvertisedAddress()
	data.Controller.Web.BindPoints.AddressPort = cmdHelper.GetCtrlEdgeAdvertisedPort()
	// Web Identities are handled in create_config_controller
	data.Controller.Web.Options.IdleTimeout = edge.DefaultHttpIdleTimeout
	data.Controller.Web.Options.ReadTimeout = edge.DefaultHttpReadTimeout
	data.Controller.Web.Options.WriteTimeout = edge.DefaultHttpWriteTimeout
	data.Controller.Web.Options.MinTLSVersion = fabXweb.ReverseTlsVersionMap[fabXweb.MinTLSVersion]
	data.Controller.Web.Options.MaxTLSVersion = fabXweb.ReverseTlsVersionMap[fabXweb.MaxTLSVersion]

	// ************* Router Values ************
	data.Router.Edge.Port = cmdHelper.GetZitiEdgeRouterPort()
	data.Router.Edge.ListenerBindPort = cmdHelper.GetZitiEdgeRouterListenerBindPort()
	data.Router.Edge.Resolver = cmdHelper.GetZitiEdgeRouterResolver()
	data.Router.Edge.DnsSvcIpRange = cmdHelper.GetZitiEdgeRouterDnsSvcIpRange()
	data.Router.Edge.CsrC = cmdHelper.GetZitiEdgeRouterC()
	data.Router.Edge.CsrST = cmdHelper.GetZitiEdgeRouterST()
	data.Router.Edge.CsrL = cmdHelper.GetZitiEdgeRouterL()
	data.Router.Edge.CsrO = cmdHelper.GetZitiEdgeRouterO()
	data.Router.Edge.CsrOU = cmdHelper.GetZitiEdgeRouterOU()
	data.Router.Edge.CsrSans = cmdHelper.GetRouterSans()
	// If CSR SANs is an IP, ignore it by setting it blank
	result, _ := regexp.MatchString("^[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}$", data.Router.Edge.CsrSans)
	if result {
		logrus.Warnf("DNS provided (%s) appears to be an IP, ignoring for DNS entry", data.Router.Edge.CsrSans)
		data.Router.Edge.CsrSans = ""
	}
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
	data.Router.Forwarder.XgressDialQueueLength = fabForwarder.DefaultXgressDialWorkerQueueLength
	data.Router.Forwarder.XgressDialWorkerCount = fabForwarder.DefaultXgressDialWorkerCount
	data.Router.Forwarder.LinkDialQueueLength = fabForwarder.DefaultLinkDialQueueLength
	data.Router.Forwarder.LinkDialWorkerCount = fabForwarder.DefaultLinkDialWorkerCount
	data.Router.Listener.OutQueueSize = channel.DefaultOutQueueSize
	data.Router.Listener.ConnectTimeout = channel.DefaultConnectTimeout
}
