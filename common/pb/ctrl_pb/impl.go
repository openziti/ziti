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

package ctrl_pb

import (
	"github.com/openziti/ziti/v2/controller/xt"
)

func (request *CircuitConfirmation) GetContentType() int32 {
	return int32(ContentType_CircuitConfirmationType)
}

func (request *RouterLinks) GetContentType() int32 {
	return int32(ContentType_RouterLinksType)
}

func (request *VerifyRouter) GetContentType() int32 {
	return int32(ContentType_VerifyRouterType)
}

func (request *Fault) GetContentType() int32 {
	return int32(ContentType_FaultType)
}

func (request *Route) GetContentType() int32 {
	return int32(ContentType_RouteType)
}

func (request *Unroute) GetContentType() int32 {
	return int32(ContentType_UnrouteType)
}

func (request *ValidateTerminatorsRequest) GetContentType() int32 {
	return int32(ContentType_ValidateTerminatorsRequestType)
}

func (request *ValidateTerminatorsV2Request) GetContentType() int32 {
	return int32(ContentType_ValidateTerminatorsV2RequestType)
}

func (request *ValidateTerminatorsV2Response) GetContentType() int32 {
	return int32(ContentType_ValidateTerminatorsV2ResponseType)
}

func (request *CircuitRequest) GetContentType() int32 {
	return int32(ContentType_CircuitRequestType)
}

func (request *RemoveTerminatorRequest) GetContentType() int32 {
	return int32(ContentType_RemoveTerminatorRequestType)
}

func (request *RemoveTerminatorsRequest) GetContentType() int32 {
	return int32(ContentType_RemoveTerminatorsRequestType)
}

func (request *InspectRequest) GetContentType() int32 {
	return int32(ContentType_InspectRequestType)
}

func (response *InspectResponse) GetContentType() int32 {
	return int32(ContentType_InspectResponseType)
}

func (request *CheckStaleLinksRequest) GetContentType() int32 {
	return int32(ContentType_CheckStaleLinksRequestType)
}

func (response *CheckStaleLinksResponse) GetContentType() int32 {
	return int32(ContentType_CheckStaleLinksResponseType)
}

func (response *InspectResponse) AddValue(name, value string) {
	newValue := &InspectResponse_InspectValue{
		Name:  name,
		Value: value,
	}
	response.Values = append(response.Values, newValue)
}

func (request *CreateTerminatorRequest) GetXtPrecedence() xt.Precedence {
	if request.GetPrecedence() == TerminatorPrecedence_Failed {
		return xt.Precedences.Failed
	}
	if request.GetPrecedence() == TerminatorPrecedence_Required {
		return xt.Precedences.Required
	}
	return xt.Precedences.Default
}

func (request *UpdateCtrlAddresses) GetContentType() int32 {
	return int32(ContentType_UpdateCtrlAddressesType)
}

func (request *PeerStateChanges) GetContentType() int32 {
	return int32(ContentType_PeerStateChangeRequestType)
}

func (request *UpdateClusterLeader) GetContentType() int32 {
	return int32(ContentType_UpdateClusterLeaderRequestType)
}

func (request *RouterInterfacesUpdate) GetContentType() int32 {
	return int32(ContentType_UpdateRouterInterfaces)
}

func (request *LinkStateUpdate) GetContentType() int32 {
	return int32(ContentType_LinkState)
}

func (request *Alerts) GetContentType() int32 {
	return int32(ContentType_AlertsType)
}

// Legacy header IDs used by pre-2.0 routers. These collide with channel library
// headers (GroupSecretHeader=10, IsFirstGroupConnection=11, UnderlayTypeHeader=12),
// but old routers don't send grouped channel headers so there's no actual conflict.
const (
	LegacyListenersHeader      int32 = 10
	LegacyRouterMetadataHeader int32 = 11
	LegacyCapabilitiesHeader   int32 = 12
)

