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

package capabilities

import (
	"github.com/openziti/sdk-golang/v2/pb/edge_client_pb"
)

// ControllerCapability is the type for capabilities the controller advertises to
// routers on the control channel. It is a separate namespace from
// RouterCapability: the controller sends a ControllerCapabilityMask on the
// control channel, routers act on the bits they recognize and ignore the rest.
type ControllerCapability int

const (
	// ControllerCreateTerminatorV2 deprecated, assumed to be supported
	// indicates support for create terminator v2
	// still advertised for pre-2.0 routers
	ControllerCreateTerminatorV2 ControllerCapability = 1

	// ControllerSingleRouterLinkSource deprecated, assumed to be supported
	// indicates that it supports routers reporting only dialed links
	// still advertised for pre-2.0 routers
	ControllerSingleRouterLinkSource ControllerCapability = 2

	// ControllerCreateCircuitV2 deprecated, assumed to be supported
	// indicates support for the CreateCircuitV2 method
	// still advertised for pre-2.0 routers
	ControllerCreateCircuitV2 ControllerCapability = 3

	// ControllerRouterDataModel deprecated, assumed to be supported
	// indicates support for the CreateCircuitV2 method
	// still advertised for pre-2.0 routers
	ControllerRouterDataModel ControllerCapability = 4

	// ControllerGroupedCtrlChan indicates support for grouped-underlay control channels
	ControllerGroupedCtrlChan ControllerCapability = 5

	// ControllerSupportsJWTLegacySessions indicates that the controller generates legacy
	// session tokens as JWTs, carrying identity and service information
	ControllerSupportsJWTLegacySessions ControllerCapability = 6
)

// RouterCapability is the type for capabilities a router advertises. Router
// capabilities form a single bit-position namespace shared across both channels a
// router advertises on: the edge channel to the SDK, and the control channel to
// the controller. The same bit means the same thing on both; each consumer acts
// on the bits it recognizes and ignores the rest.
//
// Capabilities the SDK cares about are defined once in sdk-golang as
// edge_client_pb.RouterCapability and referenced here with their (non-negative)
// bit positions, numbered upward from bit 1. RouterCapability is a distinct type
// from edge_client_pb.RouterCapability (the SDK enum); every router capability
// constant carries this type.
//
// A control-plane-only capability — one only the controller and router need, e.g.
// a quick fix that shouldn't wait on an SDK proto release — is defined here with
// a NEGATIVE value. Negative capabilities index down from the top of the mask (-1
// is the top bit; see Mask), so they never collide with the SDK's upward-numbered
// bits, and they are intentionally invisible to the SDK and the edge-api, showing
// up there as unknown bit positions. The distinct type and the sign convention
// are what let the provenance unit test scope exactly the router capabilities,
// require every non-negative one to be SDK-sourced, and verify no two overlap.
type RouterCapability int

// RouterCapabilityMask is a capability mask over the router capability namespace.
type RouterCapabilityMask = Mask[RouterCapability]

// ControllerCapabilityMask is a capability mask over the controller capability
// namespace.
type ControllerCapabilityMask = Mask[ControllerCapability]

const (
	// RouterMultiChannel indicates the router uses new (1000+) ControlHeaders IDs
	// and supports multi-underlay control channels
	RouterMultiChannel RouterCapability = RouterCapability(edge_client_pb.RouterCapability_MultiChannel)

	// RouterConnectV2 indicates the router supports the ConnectV2 message flow
	RouterConnectV2 RouterCapability = RouterCapability(edge_client_pb.RouterCapability_ConnectV2)

	// RouterPostureChecks indicates the router supports posture checks. Also
	// advertised to older SDKs via the deprecated SupportsPostureChecks edge header.
	RouterPostureChecks RouterCapability = RouterCapability(edge_client_pb.RouterCapability_PostureChecks)

	// RouterBindSuccess indicates the router sends bind-success notifications. Also
	// advertised to older SDKs via the deprecated SupportsBindSuccess edge header.
	RouterBindSuccess RouterCapability = RouterCapability(edge_client_pb.RouterCapability_BindSuccess)

	// Example of a control-plane-only capability (not advertised). A capability
	// that only the controller and router need — and that shouldn't wait on an
	// SDK proto release — is defined here with a negative value, which places it
	// at the top of the mask, invisible to the SDK and edge-api. Uncomment,
	// rename, and add it to GetRouterCapabilitiesMask to advertise one; the next
	// available slot is -1.
	//
	// RouterExampleControlOnly RouterCapability = -1
)

// GetRouterCapabilitiesMask returns the full router capability bitmask advertised
// on both the control channel and the edge channel.
func GetRouterCapabilitiesMask() *RouterCapabilityMask {
	return NewMask(
		RouterMultiChannel,
		RouterConnectV2,
		RouterPostureChecks,
		RouterBindSuccess,
	)
}

// GetControllerCapabilitiesMask returns the full controller capability bitmask
// advertised to routers on the control channel.
func GetControllerCapabilitiesMask() *ControllerCapabilityMask {
	return NewMask(
		ControllerCreateTerminatorV2,
		ControllerSingleRouterLinkSource,
		ControllerCreateCircuitV2,
		ControllerRouterDataModel,
		ControllerGroupedCtrlChan,
		ControllerSupportsJWTLegacySessions,
	)
}
