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

package constants

import "time"

const (
	ZITI                    = "ziti"
	ZITI_CONTROLLER         = "ziti-controller"
	ZITI_ROUTER             = "ziti-router"
	ZITI_TUNNEL             = "ziti-tunnel"
	ZITI_EDGE_TUNNEL        = "ziti-edge-tunnel"
	ZITI_EDGE_TUNNEL_GITHUB = "ziti-tunnel-sdk-c"
	ZITI_PROX_C             = "ziti-prox-c"
	ZITI_SDK_C_GITHUB       = "ziti-sdk-c"

	TERRAFORM_PROVIDER_PREFIX          = "terraform-provider-"
	TERRAFORM_PROVIDER_EDGE_CONTROLLER = "edgecontroller"

	CONFIGFILENAME = "config"
)

// Config Template Constants
const (
	DefaultZitiEdgeRouterListenerBindPort = "10080"
	DefaultGetSessionTimeout              = 60 * time.Second

	DefaultZitiEdgeRouterPort    = "3022"
	DefaultCtrlWebAdvertisedPort = "1280"
	DefaultCtrlListenerHostname  = "0.0.0.0"
	DefaultCtrlListenerPort      = "6262"
	DefaultCtrlMgmtListenerPort  = "10000"
)

// Env Var Constants
const (
	ZitiHomeVarName        = "ZITI_HOME"
	ZitiHomeVarDescription = "Root home directory for Ziti related files"

	CtrlIdentityCertVarName                          = "ZITI_PKI_CTRL_CERT"
	CtrlIdentityCertVarDescription                   = "Path to Identity Cert for Ziti Controller"
	CtrlIdentityServerCertVarName                    = "ZITI_PKI_CTRL_SERVER_CERT"
	CtrlIdentityServerCertVarDescription             = "Path to Identity Server Cert for Ziti Controller"
	CtrlIdentityKeyVarName                           = "ZITI_PKI_CTRL_KEY"
	CtrlIdentityKeyVarDescription                    = "Path to Identity Key for Ziti Controller"
	CtrlIdentityCAVarName                            = "ZITI_PKI_CTRL_CA"
	CtrlIdentityCAVarDescription                     = "Path to Identity CA for Ziti Controller"
	CtrlListenerHostnameVarName                      = "ZITI_CTRL_LISTENER_ADDRESS"
	CtrlListenerHostnameVarDescription               = "The hostname routers will use to connect to the Ziti Controller"
	CtrlListenerPortVarName                          = "ZITI_CTRL_LISTENER_PORT"
	CtrlListenerPortVarDescription                   = "The port routers will use to connect to the Ziti Controller"
	CtrlMgmtHostnameVarName                          = "ZITI_CTRL_MGMT_ADDRESS"
	CtrlMgmtHostnameVarDescription                   = "The hostname for the controller's management plane"
	CtrlMgmtPortVarName                              = "ZITI_CTRL_MGMT_PORT"
	CtrlMgmtPortVarDescription                       = "The port for the controller's management plane"
	CtrlEdgeApiHostnameVarName                       = "ZITI_CTRL_EDGE_API_ADDRESS"
	CtrlEdgeApiHostnameVarDescription                = "The hostname for the controller's API plane"
	CtrlEdgeApiPortVarName                           = "ZITI_CTRL_EDGE_API_PORT"
	CtrlEdgeApiPortVarDescription                    = "The port for the controller's API plane"
	CtrlSigningCertVarName                           = "ZITI_PKI_SIGNER_CERT"
	CtrlSigningCertVarDescription                    = "Path to the Ziti Signing Cert"
	CtrlSigningKeyVarName                            = "ZITI_PKI_SIGNER_KEY"
	CtrlSigningKeyVarDescription                     = "Path to the Ziti Signing Key"
	CtrlEdgeIdentityEnrollmentDurationVarName        = "ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION"
	CtrlEdgeIdentityEnrollmentDurationVarDescription = "The identity enrollment duration in minutes"
	CtrlEdgeRouterEnrollmentDurationVarName          = "ZITI_EDGE_ROUTER_ENROLLMENT_DURATION"
	CtrlEdgeRouterEnrollmentDurationVarDescription   = "The router enrollment duration in minutes"
	CtrlWebInterfaceHostnameVarName                  = "ZITI_CTRL_WEB_INTERFACE_ADDRESS"
	CtrlWebInterfaceHostnameVarDescription           = "The interface hostname to bind the controller's web listener to"
	CtrlWebInterfacePortVarName                      = "ZITI_CTRL_WEB_INTERFACE_PORT"
	CtrlWebInterfacePortVarDescription               = "The interface port to bind the controller's web listener to"
	CtrlWebAdvertisedHostnameVarName                 = "ZITI_CTRL_WEB_ADVERTISED_ADDRESS"
	CtrlWebAdvertisedHostnameVarDescription          = "The publicly addressable controller hostname value"
	CtrlWebAdvertisedPortVarName                     = "ZITI_CTRL_WEB_ADVERTISED_PORT"
	CtrlWebAdvertisedPortVarDescription              = "The publicly addressable controller port value"
	CtrlWebIdentityCertVarName                       = "ZITI_PKI_EDGE_CERT"
	CtrlEdgeIdentityCertVarDescription               = "Path to Identity Cert for Ziti Edge Controller"
	CtrlWebIdentityServerCertVarName                 = "ZITI_PKI_EDGE_SERVER_CERT"
	CtrlEdgeIdentityServerCertVarDescription         = "Path to Identity Server Cert for Ziti Edge Controller"
	CtrlWebIdentityKeyVarName                        = "ZITI_PKI_EDGE_KEY"
	CtrlEdgeIdentityKeyVarDescription                = "Path to Identity Key for Ziti Edge Controller"
	CtrlWebIdentityCAVarName                         = "ZITI_PKI_EDGE_CA"
	CtrlEdgeIdentityCAVarDescription                 = "Path to Identity CA for Ziti Edge Controller"

	ZitiEdgeRouterRawNameVarName                 = "ZITI_EDGE_ROUTER_NAME"
	ZitiEdgeRouterRawNameVarDescription          = "The Edge Router Raw Name"
	ZitiEdgeRouterPortVarName                    = "ZITI_EDGE_ROUTER_PORT"
	ZitiEdgeRouterPortVarDescription             = "Port of the Ziti Edge Router"
	ZitiRouterIdentityCertVarName                = "ZITI_ROUTER_IDENTITY_CERT"
	ZitiRouterIdentityCertVarDescription         = "Path to Identity Cert for Ziti Router"
	ZitiRouterIdentityServerCertVarName          = "ZITI_ROUTER_IDENTITY_SERVER_CERT"
	ZitiRouterIdentityServerCertVarDescription   = "Path to Identity Server Cert for Ziti Router"
	ZitiRouterIdentityKeyVarName                 = "ZITI_ROUTER_IDENTITY_KEY"
	ZitiRouterIdentityKeyVarDescription          = "Path to Identity Key for Ziti Router"
	ZitiRouterIdentityCAVarName                  = "ZITI_ROUTER_IDENTITY_CA"
	ZitiRouterIdentityCAVarDescription           = "Path to Identity CA for Ziti Router"
	ZitiEdgeRouterIPOverrideVarName              = "ZITI_EDGE_ROUTER_IP_OVERRIDE"
	ZitiEdgeRouterIPOverrideVarDescription       = "Override the default edge router IP with a custom IP, this IP will also be added to the PKI"
	ZitiEdgeRouterAdvertisedHostVarName          = "ZITI_EDGE_ROUTER_ADVERTISED_HOST"
	ZitiEdgeRouterAdvertisedHostVarDescription   = "The advertised host of the router"
	ZitiEdgeRouterListenerBindPortVarName        = "ZITI_EDGE_ROUTER_LISTENER_BIND_PORT"
	ZitiEdgeRouterListenerBindPortVarDescription = "The port a public router will advertise on"
	ExternalDNSVarName                           = "EXTERNAL_DNS"
)
