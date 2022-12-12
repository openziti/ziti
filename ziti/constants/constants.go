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

	DefaultZitiEdgeRouterPort = "3022"

	DefaultZitiControllerName            = "controller"
	DefaultZitiEdgeAPIPort               = "1280"
	DefaultZitiEdgeListenerHost          = "0.0.0.0"
	DefaultZitiControllerListenerAddress = "0.0.0.0"
	DefaultZitiControllerPort            = "6262"
)

// Env Var Constants
const (
	ZitiHomeVarName                                  = "ZITI_HOME"
	ZitiHomeVarDescription                           = "Root home directory for Ziti related files"
	ZitiCtrlNameVarName                              = "ZITI_CONTROLLER_NAME"
	ZitiCtrlNameVarDescription                       = "The name of the Ziti Controller"
	ZitiCtrlPortVarName                              = "ZITI_CTRL_PORT"
	ZitiCtrlPortVarDescription                       = "The port of the Ziti Controller"
	ZitiEdgeRouterRawNameVarName                     = "ZITI_EDGE_ROUTER_RAWNAME"
	ZitiEdgeRouterRawNameVarDescription              = "The Edge Router Raw Name"
	ZitiEdgeRouterPortVarName                        = "ZITI_EDGE_ROUTER_PORT"
	ZitiEdgeRouterPortVarDescription                 = "Port of the Ziti Edge Router"
	ZitiEdgeCtrlIdentityCertVarName                  = "ZITI_EDGE_CTRL_IDENTITY_CERT"
	ZitiEdgeCtrlIdentityCertVarDescription           = "Path to Identity Cert for Ziti Edge Controller"
	ZitiEdgeCtrlIdentityServerCertVarName            = "ZITI_EDGE_CTRL_IDENTITY_SERVER_CERT"
	ZitiEdgeCtrlIdentityServerCertVarDescription     = "Path to Identity Server Cert for Ziti Edge Controller"
	ZitiEdgeCtrlIdentityKeyVarName                   = "ZITI_EDGE_CTRL_IDENTITY_KEY"
	ZitiEdgeCtrlIdentityKeyVarDescription            = "Path to Identity Key for Ziti Edge Controller"
	ZitiEdgeCtrlIdentityCAVarName                    = "ZITI_EDGE_CTRL_IDENTITY_CA"
	ZitiEdgeCtrlIdentityCAVarDescription             = "Path to Identity CA for Ziti Edge Controller"
	ZitiCtrlIdentityCertVarName                      = "ZITI_CTRL_IDENTITY_CERT"
	ZitiCtrlIdentityCertVarDescription               = "Path to Identity Cert for Ziti Controller"
	ZitiCtrlIdentityServerCertVarName                = "ZITI_CTRL_IDENTITY_SERVER_CERT"
	ZitiCtrlIdentityServerCertVarDescription         = "Path to Identity Server Cert for Ziti Controller"
	ZitiCtrlIdentityKeyVarName                       = "ZITI_CTRL_IDENTITY_KEY"
	ZitiCtrlIdentityKeyVarDescription                = "Path to Identity Key for Ziti Controller"
	ZitiCtrlIdentityCAVarName                        = "ZITI_CTRL_IDENTITY_CA"
	ZitiCtrlIdentityCAVarDescription                 = "Path to Identity CA for Ziti Controller"
	ZitiSigningCertVarName                           = "ZITI_SIGNING_CERT"
	ZitiSigningCertVarDescription                    = "Path to the Ziti Signing Cert"
	ZitiSigningKeyVarName                            = "ZITI_SIGNING_KEY"
	ZitiSigningKeyVarDescription                     = "Path to the Ziti Signing Key"
	ZitiRouterIdentityCertVarName                    = "ZITI_ROUTER_IDENTITY_CERT"
	ZitiRouterIdentityCertVarDescription             = "Path to Identity Cert for Ziti Router"
	ZitiRouterIdentityServerCertVarName              = "ZITI_ROUTER_IDENTITY_SERVER_CERT"
	ZitiRouterIdentityServerCertVarDescription       = "Path to Identity Server Cert for Ziti Router"
	ZitiRouterIdentityKeyVarName                     = "ZITI_ROUTER_IDENTITY_KEY"
	ZitiRouterIdentityKeyVarDescription              = "Path to Identity Key for Ziti Router"
	ZitiRouterIdentityCAVarName                      = "ZITI_ROUTER_IDENTITY_CA"
	ZitiRouterIdentityCAVarDescription               = "Path to Identity CA for Ziti Router"
	ZitiCtrlListenerAddressVarName                   = "ZITI_CTRL_LISTENER_ADDRESS"
	ZitiCtrlListenerAddressVarDescription            = "The Ziti Controller Listener Address"
	ZitiCtrlAdvertisedAddressVarName                 = "ZITI_CTRL_ADVERTISED_ADDRESS"
	ZitiCtrlAdvertisedAddressVarDescription          = "The Ziti Controller Advertised Address"
	ZitiEdgeCtrlListenerHostPortVarName              = "ZITI_CTRL_EDGE_LISTENER_HOST_PORT"
	ZitiEdgeCtrlListenerHostPortVarDescription       = "Host and port of the Ziti Edge Controller Listener"
	ZitiEdgeCtrlAdvertisedHostPortVarName            = "ZITI_EDGE_CTRL_ADVERTISED_HOST_PORT"
	ZitiEdgeCtrlAdvertisedHostPortVarDescription     = "Host and port of the Ziti Edge Controller API"
	ZitiEdgeCtrlAdvertisedPortVarName                = "ZITI_EDGE_CONTROLLER_PORT"
	ZitiEdgeCtrlAdvertisedPortVarDescription         = "The advertised port of the edge controller"
	ZitiEdgeIdentityEnrollmentDurationVarName        = "ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION"
	ZitiEdgeIdentityEnrollmentDurationVarDescription = "The identity enrollment duration in minutes"
	ZitiEdgeRouterEnrollmentDurationVarName          = "ZITI_EDGE_ROUTER_ENROLLMENT_DURATION"
	ZitiEdgeRouterEnrollmentDurationVarDescription   = "The router enrollment duration in minutes"
	ExternalDNSVarName                               = "EXTERNAL_DNS"
	ZitiEdgeRouterIPOverrideVarName                  = "ZITI_EDGE_ROUTER_IP_OVERRIDE"
	ZitiEdgeRouterIPOverrideVarDescription           = "Override the default edge router IP with a custom IP, this IP will also be added to the PKI"
	ZitiEdgeRouterAdvertisedHostVarName              = "ZITI_EDGE_ROUTER_ADVERTISED_HOST"
	ZitiEdgeRouterAdvertisedHostVarDescription       = "The advertised host of the router"
	ZitiEdgeRouterListenerBindPortVarName            = "ZITI_EDGE_ROUTER_LISTENER_BIND_PORT"
	ZitiEdgeRouterListenerBindPortVarDescription     = "The port a public router will advertise on"
)
