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

	DefaultZitiEdgeRouterPort     = "3022"
	DefaultCtrlEdgeAdvertisedPort = "1280"
	DefaultCtrlListenerAddress    = "0.0.0.0"
	DefaultCtrlListenerPort       = "6262"
)

// Env Var Constants
const (
	ZitiHomeVarName        = "ZITI_HOME"
	ZitiHomeVarDescription = "Root home directory for Ziti related files"

	PkiCtrlCertVarName                               = "ZITI_PKI_CTRL_CERT"
	PkiCtrlCertVarDescription                        = "Path to Identity Cert for Ziti Controller"
	PkiCtrlServerCertVarName                         = "ZITI_PKI_CTRL_SERVER_CERT"
	PkiCtrlServerCertVarDescription                  = "Path to Identity Server Cert for Ziti Controller"
	PkiCtrlKeyVarName                                = "ZITI_PKI_CTRL_KEY"
	PkiCtrlKeyVarDescription                         = "Path to Identity Key for Ziti Controller"
	PkiCtrlCAVarName                                 = "ZITI_PKI_CTRL_CA"
	PkiCtrlCAVarDescription                          = "Path to Identity CA for Ziti Controller"
	CtrlListenerAddressVarName                       = "ZITI_CTRL_LISTENER_ADDRESS"
	CtrlListenerAddressVarDescription                = "The hostname routers will use to connect to the Ziti Controller"
	CtrlListenerPortVarName                          = "ZITI_CTRL_LISTENER_PORT"
	CtrlListenerPortVarDescription                   = "The port routers will use to connect to the Ziti Controller"
	CtrlEdgeApiAddressVarName                        = "ZITI_CTRL_EDGE_API_ADDRESS"
	CtrlEdgeApiAddressVarDescription                 = "The hostname for the controller's API plane"
	CtrlEdgeApiPortVarName                           = "ZITI_CTRL_EDGE_API_PORT"
	CtrlEdgeApiPortVarDescription                    = "The port for the controller's API plane"
	PkiSignerCertVarName                             = "ZITI_PKI_SIGNER_CERT"
	PkiSignerCertVarDescription                      = "Path to the Ziti Signing Cert"
	PkiSignerKeyVarName                              = "ZITI_PKI_SIGNER_KEY"
	PkiSignerKeyVarDescription                       = "Path to the Ziti Signing Key"
	CtrlEdgeIdentityEnrollmentDurationVarName        = "ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION"
	CtrlEdgeIdentityEnrollmentDurationVarDescription = "The identity enrollment duration in minutes"
	CtrlEdgeRouterEnrollmentDurationVarName          = "ZITI_EDGE_ROUTER_ENROLLMENT_DURATION"
	CtrlEdgeRouterEnrollmentDurationVarDescription   = "The router enrollment duration in minutes"
	CtrlEdgeInterfaceAddressVarName                  = "ZITI_CTRL_EDGE_INTERFACE_ADDRESS"
	CtrlEdgeInterfaceAddressVarDescription           = "The interface hostname to bind the controller's edge listener to"
	CtrlEdgeInterfacePortVarName                     = "ZITI_CTRL_EDGE_INTERFACE_PORT"
	CtrlEdgeInterfacePortVarDescription              = "The interface port to bind the controller's edge listener to"
	CtrlEdgeAdvertisedAddressVarName                 = "ZITI_CTRL_EDGE_ADVERTISED_ADDRESS"
	CtrlEdgeAdvertisedAddressVarDescription          = "The publicly addressable controller address value"
	CtrlEdgeAdvertisedPortVarName                    = "ZITI_CTRL_EDGE_ADVERTISED_PORT"
	CtrlEdgeAdvertisedPortVarDescription             = "The publicly addressable controller port value"
	CtrlPkiEdgeCertVarName                           = "ZITI_PKI_EDGE_CERT"
	CtrlPkiEdgeCertVarDescription                    = "Path to Identity Cert for Ziti Edge Controller"
	CtrlPkiEdgeServerCertVarName                     = "ZITI_PKI_EDGE_SERVER_CERT"
	CtrlPkiEdgeServerCertVarDescription              = "Path to Identity Server Cert for Ziti Edge Controller"
	CtrlPkiEdgeKeyVarName                            = "ZITI_PKI_EDGE_KEY"
	CtrlPkiEdgeKeyVarDescription                     = "Path to Identity Key for Ziti Edge Controller"
	CtrlPkiEdgeCAVarName                             = "ZITI_PKI_EDGE_CA"
	CtrlPkiEdgeCAVarDescription                      = "Path to Identity CA for Ziti Edge Controller"

	ZitiEdgeRouterNameVarName                    = "ZITI_EDGE_ROUTER_NAME"
	ZitiEdgeRouterNameVarDescription             = "The Edge Router Raw Name"
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
