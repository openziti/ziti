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

import "math/big"

const (
	// ControllerCreateTerminatorV2 deprecated, assumed to be supported
	// indicates support for create terminator v2
	// still advertised for pre-2.0 routers
	ControllerCreateTerminatorV2 int = 1

	// ControllerSingleRouterLinkSource deprecated, assumed to be supported
	// indicates that it supports routers reporting only dialed links
	// still advertised for pre-2.0 routers
	ControllerSingleRouterLinkSource int = 2

	// ControllerCreateCircuitV2 deprecated, assumed to be supported
	// indicates support for the CreateCircuitV2 method
	// still advertised for pre-2.0 routers
	ControllerCreateCircuitV2 int = 3

	// ControllerRouterDataModel deprecated, assumed to be supported
	// indicates support for the CreateCircuitV2 method
	// still advertised for pre-2.0 routers
	ControllerRouterDataModel int = 4

	// ControllerGroupedCtrlChan indicates support for grouped-underlay control channels
	ControllerGroupedCtrlChan int = 5

	// ControllerSupportsJWTLegacySessions indicates that the controller generates legacy
	// session tokens as JWTs, carrying identity and service information
	ControllerSupportsJWTLegacySessions int = 6

	// ControllerLinkGossip indicates the controller distributes link state via gossip.
	// Routers seeing this on all connected controllers may report links to a single
	// controller instead of all.
	ControllerLinkGossip int = 7
)

// Router Capabilities
const (
	// RouterMultiChannel indicates the router uses new (1000+) ControlHeaders IDs
	// and supports multi-underlay control channels
	RouterMultiChannel int = 1
)

func GetControllerCapabilitiesMask() *big.Int {
	capabilityMask := &big.Int{}
	capabilityMask.SetBit(capabilityMask, ControllerCreateTerminatorV2, 1)
	capabilityMask.SetBit(capabilityMask, ControllerSingleRouterLinkSource, 1)
	capabilityMask.SetBit(capabilityMask, ControllerCreateCircuitV2, 1)
	capabilityMask.SetBit(capabilityMask, ControllerRouterDataModel, 1)
	capabilityMask.SetBit(capabilityMask, ControllerGroupedCtrlChan, 1)
	capabilityMask.SetBit(capabilityMask, ControllerSupportsJWTLegacySessions, 1)
	capabilityMask.SetBit(capabilityMask, ControllerLinkGossip, 1)
	return capabilityMask
}
