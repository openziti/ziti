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
	OpenZitiOrg             = "openziti"
	ZITI                    = "ziti"
	ZROK                    = "zrok"
	CaddyOrg                = "caddyserver"
	Caddy                   = "caddy"
	ZITI_CONTROLLER         = "ziti-controller"
	ZITI_ROUTER             = "ziti-router"
	ZITI_TUNNEL             = "ziti-tunnel"
	ZITI_EDGE_TUNNEL        = "ziti-edge-tunnel"
	ZITI_EDGE_TUNNEL_GITHUB = "ziti-tunnel-sdk-c"
)

// Config Template Constants
const (
	DefaultZitiEdgeRouterListenerBindPort = "10080"
	DefaultGetSessionTimeout              = 60 * time.Second

	DefaultZitiEdgeRouterPort = "3022"

	DefaultCtrlBindAddress    = "0.0.0.0"
	DefaultCtrlAdvertisedPort = "6262"

	DefaultCtrlDatabaseFile = "db/ctrl.db"

	DefaultCtrlEdgeBindAddress    = "0.0.0.0"
	DefaultCtrlEdgeAdvertisedPort = "1280"

	DefaultEdgeRouterCsrC  = "US"
	DefaultEdgeRouterCsrST = "NC"
	DefaultEdgeRouterCsrL  = "Charlotte"
	DefaultEdgeRouterCsrO  = "NetFoundry"
	DefaultEdgeRouterCsrOU = "Ziti"
)

// Env Var Constants
const (
	ZitiHomeVarName        = "ZITI_HOME"
	ZitiHomeVarDescription = "base dirname used to construct paths"

	ZitiNetworkNameVarName        = "ZITI_NETWORK_NAME"
	ZitiNetworkNameVarDescription = "base filename used to construct paths"

	PkiCtrlCertVarName                               = "ZITI_PKI_CTRL_CERT"
	PkiCtrlCertVarDescription                        = "Path to the controller's default identity client cert"
	PkiCtrlServerCertVarName                         = "ZITI_PKI_CTRL_SERVER_CERT"
	PkiCtrlServerCertVarDescription                  = "Path to the controller's default identity server cert, including partial chain"
	PkiCtrlKeyVarName                                = "ZITI_PKI_CTRL_KEY"
	PkiCtrlKeyVarDescription                         = "Path to the controller's default identity private key"
	PkiCtrlCAVarName                                 = "ZITI_PKI_CTRL_CA"
	PkiCtrlCAVarDescription                          = "Path to the controller's bundle of trusted root CAs"
	CtrlBindAddressVarName                           = "ZITI_CTRL_BIND_ADDRESS"
	CtrlBindAddressVarDescription                    = "The address where the controller will listen for router control plane connections"
	CtrlAdvertisedAddressVarName                     = "ZITI_CTRL_ADVERTISED_ADDRESS"
	CtrlAdvertisedAddressVarDescription              = "The address routers will use to connect to the controller"
	CtrlAdvertisedPortVarName                        = "ZITI_CTRL_ADVERTISED_PORT"
	CtrlAdvertisedPortVarDescription                 = "TCP port routers will use to connect to the controller"
	CtrlConsoleLocationVarName                       = "ZITI_CTRL_CONSOLE_LOCATION"
	CtrlConsoleLocationVarDescription                = "The filesystem path of the controller's web console"
	CtrlEdgeBindAddressVarName                       = "ZITI_CTRL_EDGE_BIND_ADDRESS"
	CtrlEdgeBindAddressVarDescription                = "The address where the controller will listen for edge API connections"
	CtrlEdgeAdvertisedAddressVarName                 = "ZITI_CTRL_EDGE_ADVERTISED_ADDRESS"
	CtrlEdgeAdvertisedAddressVarDescription          = "The controller's edge API address"
	CtrlEdgeAltAdvertisedAddressVarName              = "ZITI_CTRL_EDGE_ALT_ADVERTISED_ADDRESS"
	CtrlEdgeAltAdvertisedAddressVarDescription       = "The controller's edge API alternative address"
	CtrlEdgeAdvertisedPortVarName                    = "ZITI_CTRL_EDGE_ADVERTISED_PORT"
	CtrlEdgeAdvertisedPortVarDescription             = "TCP port of the controller's edge API"
	CtrlDatabaseFileVarName                          = "ZITI_CTRL_DATABASE_FILE"
	CtrlDatabaseFileVarDescription                   = "Path to the controller's database file"
	PkiSignerCertVarName                             = "ZITI_PKI_SIGNER_CERT"
	PkiSignerCertVarDescription                      = "Path to the controller's edge signer CA cert"
	PkiSignerKeyVarName                              = "ZITI_PKI_SIGNER_KEY"
	PkiSignerKeyVarDescription                       = "Path to the controller's edge signer CA key"
	CtrlEdgeIdentityEnrollmentDurationVarName        = "ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION"
	CtrlEdgeIdentityEnrollmentDurationVarDescription = "The identity enrollment duration in minutes"
	CtrlEdgeRouterEnrollmentDurationVarName          = "ZITI_ROUTER_ENROLLMENT_DURATION"
	CtrlEdgeRouterEnrollmentDurationVarDescription   = "The router enrollment duration in minutes"
	CtrlPkiEdgeCertVarName                           = "ZITI_PKI_EDGE_CERT"
	CtrlPkiEdgeCertVarDescription                    = "Path to the controller's web identity client certificate"
	CtrlPkiEdgeServerCertVarName                     = "ZITI_PKI_EDGE_SERVER_CERT"
	CtrlPkiEdgeServerCertVarDescription              = "Path to the controller's web identity server certificate, including partial chain"
	CtrlPkiEdgeKeyVarName                            = "ZITI_PKI_EDGE_KEY"
	CtrlPkiEdgeKeyVarDescription                     = "Path to the controller's web identity private key"
	CtrlPkiEdgeCAVarName                             = "ZITI_PKI_EDGE_CA"
	CtrlPkiEdgeCAVarDescription                      = "Path to the controller's web identity root CA cert"
	PkiAltServerCertVarName                          = "ZITI_PKI_ALT_SERVER_CERT"
	PkiAltServerCertVarDescription                   = "Path to the controller's default identity alternative server certificate; requires ZITI_PKI_ALT_SERVER_KEY"
	PkiAltServerKeyVarName                           = "ZITI_PKI_ALT_SERVER_KEY"
	PkiAltServerKeyVarDescription                    = "Path to the controller's default identity alternative private key. Requires ZITI_PKI_ALT_SERVER_CERT"
	ZitiEdgeRouterNameVarName                        = "ZITI_ROUTER_NAME"
	ZitiEdgeRouterNameVarDescription                 = "A filename prefix for the router's key and certs"
	ZitiEdgeRouterPortVarName                        = "ZITI_ROUTER_PORT"
	ZitiEdgeRouterPortVarDescription                 = "TCP port where the router listens for edge connections from endpoint identities"
	ZitiRouterIdentityCertVarName                    = "ZITI_ROUTER_IDENTITY_CERT"
	ZitiRouterIdentityCertVarDescription             = "Path to the router's client certificate"
	ZitiRouterIdentityServerCertVarName              = "ZITI_ROUTER_IDENTITY_SERVER_CERT"
	ZitiRouterIdentityServerCertVarDescription       = "Path to the router's server certificate"
	ZitiRouterIdentityKeyVarName                     = "ZITI_ROUTER_IDENTITY_KEY"
	ZitiRouterIdentityKeyVarDescription              = "Path to the router's private key"
	ZitiRouterIdentityCAVarName                      = "ZITI_ROUTER_IDENTITY_CA"
	ZitiRouterIdentityCAVarDescription               = "Path to the router's bundle of trusted root CA certs"
	ZitiEdgeRouterIPOverrideVarName                  = "ZITI_ROUTER_IP_OVERRIDE"
	ZitiEdgeRouterIPOverrideVarDescription           = "Additional IP SAN of the router"
	ZitiEdgeRouterAdvertisedAddressVarName           = "ZITI_ROUTER_ADVERTISED_ADDRESS"
	ZitiEdgeRouterAdvertisedAddressVarDescription    = "The router's advertised address and DNS SAN"
	ZitiEdgeRouterListenerBindPortVarName            = "ZITI_ROUTER_LISTENER_BIND_PORT"
	ZitiEdgeRouterListenerBindPortVarDescription     = "TCP port where the router will listen for and advertise links to other routers"
	ZitiEdgeRouterResolverVarName                    = "ZITI_ROUTER_TPROXY_RESOLVER"
	ZitiEdgeRouterResolverVarDescription             = "The bind URI to listen for DNS requests in tproxy mode"
	ZitiEdgeRouterDnsSvcIpRangeVarName               = "ZITI_ROUTER_DNS_IP_RANGE"
	ZitiEdgeRouterDnsSvcIpRangeVarDescription        = "The CIDR range to use for Ziti DNS in tproxy mode"
	ZitiEdgeRouterCsrCVarName                        = "ZITI_ROUTER_CSR_C"
	ZitiEdgeRouterCsrCVarDescription                 = "The country (C) to use for router CSRs"
	ZitiEdgeRouterCsrSTVarName                       = "ZITI_ROUTER_CSR_ST"
	ZitiEdgeRouterCsrSTVarDescription                = "The state/province (ST) to use for router CSRs"
	ZitiEdgeRouterCsrLVarName                        = "ZITI_ROUTER_CSR_L"
	ZitiEdgeRouterCsrLVarDescription                 = "The locality (L) to use for router CSRs"
	ZitiEdgeRouterCsrOVarName                        = "ZITI_ROUTER_CSR_O"
	ZitiEdgeRouterCsrOVarDescription                 = "The organization (O) to use for router CSRs"
	ZitiEdgeRouterCsrOUVarName                       = "ZITI_ROUTER_CSR_OU"
	ZitiEdgeRouterCsrOUVarDescription                = "The organization unit to use for router CSRs"
	ZitiRouterCsrSansDnsVarName                      = "ZITI_ROUTER_CSR_SANS_DNS"
	ZitiRouterCsrSansDnsVarDescription               = "Additional DNS SAN of the router"
)
