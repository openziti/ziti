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

package constants

const (
	ZITI                    = "ziti"
	ZITI_CONTROLLER         = "ziti-controller"
	ZITI_FABRIC             = "ziti-fabric"
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
	DefaultListenerBindPort   = 10080
	DefaultOutQueueSize       = 16
	DefaultConnectTimeoutMs   = 5000
	DefaultGetSessionTimeoutS = 60

	// Router defaults
	DefaultZitiEdgeRouterPort = "3022"

	// Controller defaults
	DefaultEdgeAPISessionTimeoutMinutes     = 30
	DefaultWebListenerIdleTimeoutMs         = 5000
	DefaultWebListenerReadTimeoutMs         = 5000
	DefaultWebListenerWriteTimeoutMs        = 100000
	DefaultWebListenerMinTLSVersion         = "TLS1.2"
	DefaultWebListenerMaxTLSVersion         = "TLS1.3"
	DefaultZitiEdgeControllerPort           = 1280
	DefaultZitiFabricControllerPort         = 6262
	DefaultZitiFabricManagementPort         = 10000
	DefaultControllerHealthCheckIntervalSec = 30
	DefaultControllerHealthCheckTimeoutSec  = 15
	DefaultControllerHealthCheckDelaySec    = 15

	// WSS defaults
	DefaultWSSWriteTimeout     = 10
	DefaultWSSReadTimeout      = 5
	DefaultWSSIdleTimeout      = 5
	DefaultWSSPongTimeout      = 60
	DefaultWSSPingInterval     = 54
	DefaultWSSHandshakeTimeout = 10
	DefaultWSSReadBufferSize   = 4096
	DefaultWSSWriteBufferSize  = 4096

	// Forwarder defaults
	DefaultLatencyProbeInterval  = 1000
	DefaultXgressDialQueueLength = 1000
	DefaultXgressDialWorkerCount = 128
	DefaultLinkDialQueueLength   = 1000
	DefaultLinkDialWorkerCount   = 10
)
