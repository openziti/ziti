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

package ctrl_msg

import (
	"errors"
	"fmt"

	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
)

const (
	CircuitSuccessType = 1001
	CircuitFailedType  = 1016
	RouteResultType    = 1022

	CircuitSuccessAddressHeader = 1100
	RouteResultAttemptHeader    = 1101
	RouteResultSuccessHeader    = 1102
	RouteResultErrorHeader      = 1103
	RouteResultErrorCodeHeader  = 1104

	TerminatorLocalAddressHeader  = 1110
	TerminatorRemoteAddressHeader = 1111

	InitiatorLocalAddressHeader  = 1112
	InitiatorRemoteAddressHeader = 1113

	XtStickinessToken = 1114

	DialerIdentityIdHeader   = 1115
	DialerIdentityNameHeader = 1116

	ErrorTypeGeneric                 = 0
	ErrorTypeInvalidTerminator       = 1
	ErrorTypeMisconfiguredTerminator = 2
	ErrorTypeDialTimedOut            = 3
	ErrorTypeConnectionRefused       = 4
	ErrorTypeRejectedByApplication   = 5
	ErrorTypeDnsResolutionFailed     = 6
	ErrorTypePortNotAllowed          = 7
	ErrorTypeInvalidLinkDestination  = 8
	ErrorTypeResourcesNotAvailable   = 9

	CreateCircuitPeerDataHeader = 10

	CreateCircuitReqSessionTokenHeader         = 11
	CreateCircuitReqFingerprintsHeader         = 12
	CreateCircuitReqTerminatorInstanceIdHeader = 13
	CreateCircuitReqApiSessionTokenHeader      = 14

	CreateCircuitRespCircuitId  = 11
	CreateCircuitRespAddress    = 12
	CreateCircuitRespTagsHeader = 13

	HeaderResultErrorCode = 10

	ResultErrorRateLimited = 1

	CreateCircuitV3ReqIdentityIdHeader = 15
	CreateCircuitV3ReqServiceIdHeader  = 16
	CreateCircuitV3ReqCircuitIdHeader  = 17
)

func NewCircuitSuccessMsg(sessionId, address string) *channel.Message {
	msg := channel.NewMessage(CircuitSuccessType, []byte(sessionId))
	msg.Headers[CircuitSuccessAddressHeader] = []byte(address)
	return msg
}

func NewCircuitFailedMsg(message string) *channel.Message {
	return channel.NewMessage(CircuitFailedType, []byte(message))
}

func NewRouteResultSuccessMsg(sessionId string, attempt int) *channel.Message {
	msg := channel.NewMessage(RouteResultType, []byte(sessionId))
	msg.PutUint32Header(RouteResultAttemptHeader, uint32(attempt))
	msg.PutBoolHeader(RouteResultSuccessHeader, true)
	return msg
}

func NewRouteResultFailedMessage(sessionId string, attempt int, rerr string) *channel.Message {
	msg := channel.NewMessage(RouteResultType, []byte(sessionId))
	msg.PutUint32Header(RouteResultAttemptHeader, uint32(attempt))
	msg.Headers[RouteResultErrorHeader] = []byte(rerr)
	return msg
}

type CreateCircuitV2Request struct {
	ApiSessionToken      string
	SessionToken         string
	Fingerprints         []string
	TerminatorInstanceId string
	PeerData             map[uint32][]byte
}

func (self *CreateCircuitV2Request) GetApiSessionToken() string {
	return self.ApiSessionToken
}

func (self *CreateCircuitV2Request) GetSessionToken() string {
	return self.SessionToken
}

func (self *CreateCircuitV2Request) GetFingerprints() []string {
	return self.Fingerprints
}

func (self *CreateCircuitV2Request) GetTerminatorInstanceId() string {
	return self.TerminatorInstanceId
}

func (self *CreateCircuitV2Request) GetPeerData() map[uint32][]byte {
	return self.PeerData
}

func (self *CreateCircuitV2Request) ToMessage() *channel.Message {
	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_CreateCircuitV2RequestType), nil)
	msg.PutStringHeader(CreateCircuitReqSessionTokenHeader, self.SessionToken)
	msg.PutStringHeader(CreateCircuitReqApiSessionTokenHeader, self.ApiSessionToken)
	msg.PutStringSliceHeader(CreateCircuitReqFingerprintsHeader, self.Fingerprints)
	msg.PutStringHeader(CreateCircuitReqTerminatorInstanceIdHeader, self.TerminatorInstanceId)
	msg.PutU32ToBytesMapHeader(CreateCircuitPeerDataHeader, self.PeerData)
	return msg
}

func DecodeCreateCircuitV2Request(m *channel.Message) (*CreateCircuitV2Request, error) {
	sessionToken, _ := m.GetStringHeader(CreateCircuitReqSessionTokenHeader)
	if len(sessionToken) == 0 {
		return nil, errors.New("no session token provided in create circuit request")
	}

	apiSessionToken, _ := m.GetStringHeader(CreateCircuitReqApiSessionTokenHeader)

	fingerprints, _, err := m.GetStringSliceHeader(CreateCircuitReqFingerprintsHeader)
	if err != nil {
		return nil, fmt.Errorf("unable to get create circuit request fingerprints (%w)", err)
	}

	terminatorInstanceId, _ := m.GetStringHeader(CreateCircuitReqTerminatorInstanceIdHeader)
	peerData, _, err := m.GetU32ToBytesMapHeader(CreateCircuitPeerDataHeader)
	if err != nil {
		return nil, fmt.Errorf("unable to get create circuit request peer data (%w)", err)
	}

	return &CreateCircuitV2Request{
		ApiSessionToken:      apiSessionToken,
		SessionToken:         sessionToken,
		Fingerprints:         fingerprints,
		TerminatorInstanceId: terminatorInstanceId,
		PeerData:             peerData,
	}, nil
}

type CreateCircuitV2Response struct {
	CircuitId string
	Address   string
	PeerData  map[uint32][]byte
	Tags      map[string]string
}

func (self *CreateCircuitV2Response) ToMessage() *channel.Message {
	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_CreateCircuitV2ResponseType), nil)
	msg.PutStringHeader(CreateCircuitRespCircuitId, self.CircuitId)
	msg.PutStringHeader(CreateCircuitRespAddress, self.Address)
	msg.PutU32ToBytesMapHeader(CreateCircuitPeerDataHeader, self.PeerData)
	msg.PutStringToStringMapHeader(CreateCircuitRespTagsHeader, self.Tags)
	return msg
}

func DecodeCreateCircuitV2Response(m *channel.Message) (*CreateCircuitV2Response, error) {
	circuitId, _ := m.GetStringHeader(CreateCircuitRespCircuitId)
	address, _ := m.GetStringHeader(CreateCircuitRespAddress)
	peerData, _, err := m.GetU32ToBytesMapHeader(CreateCircuitPeerDataHeader)
	if err != nil {
		return nil, fmt.Errorf("unable to get create circuit response peer data (%w)", err)
	}

	tags, _, err := m.GetStringToStringMapHeader(CreateCircuitRespTagsHeader)
	if err != nil {
		return nil, fmt.Errorf("unable to get create circuit response tags (%w)", err)
	}

	return &CreateCircuitV2Response{
		CircuitId: circuitId,
		Address:   address,
		PeerData:  peerData,
		Tags:      tags,
	}, nil
}

// CreateCircuitV3Request is sent from a router to the controller to create a circuit
// without a service session token. The router has already authorized the dial locally
// via RDM and provides the identity and service IDs directly, along with a pre-assigned
// circuit ID.
type CreateCircuitV3Request struct {
	IdentityId           string
	ServiceId            string
	CircuitId            string
	Fingerprints         []string
	TerminatorInstanceId string
	PeerData             map[uint32][]byte
	ApiSessionToken      string
}

func (self *CreateCircuitV3Request) GetApiSessionToken() string {
	return self.ApiSessionToken
}

func (self *CreateCircuitV3Request) GetSessionToken() string {
	return ""
}

func (self *CreateCircuitV3Request) GetFingerprints() []string {
	return self.Fingerprints
}

func (self *CreateCircuitV3Request) GetTerminatorInstanceId() string {
	return self.TerminatorInstanceId
}

func (self *CreateCircuitV3Request) GetPeerData() map[uint32][]byte {
	return self.PeerData
}

func (self *CreateCircuitV3Request) ToMessage() *channel.Message {
	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_CreateCircuitV3RequestType), nil)
	msg.PutStringHeader(CreateCircuitV3ReqIdentityIdHeader, self.IdentityId)
	msg.PutStringHeader(CreateCircuitV3ReqServiceIdHeader, self.ServiceId)
	msg.PutStringHeader(CreateCircuitV3ReqCircuitIdHeader, self.CircuitId)
	msg.PutStringHeader(CreateCircuitReqApiSessionTokenHeader, self.ApiSessionToken)
	msg.PutStringSliceHeader(CreateCircuitReqFingerprintsHeader, self.Fingerprints)
	msg.PutStringHeader(CreateCircuitReqTerminatorInstanceIdHeader, self.TerminatorInstanceId)
	msg.PutU32ToBytesMapHeader(CreateCircuitPeerDataHeader, self.PeerData)
	return msg
}

// CreateCircuitV3Response is the response to a CreateCircuitV3Request. It carries the same
// fields as CreateCircuitV2Response but uses the CreateCircuitV3ResponseType content type.
type CreateCircuitV3Response struct {
	CircuitId string
	Address   string
	PeerData  map[uint32][]byte
	Tags      map[string]string
}

func (self *CreateCircuitV3Response) ToMessage() *channel.Message {
	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_CreateCircuitV3ResponseType), nil)
	msg.PutStringHeader(CreateCircuitRespCircuitId, self.CircuitId)
	msg.PutStringHeader(CreateCircuitRespAddress, self.Address)
	msg.PutU32ToBytesMapHeader(CreateCircuitPeerDataHeader, self.PeerData)
	msg.PutStringToStringMapHeader(CreateCircuitRespTagsHeader, self.Tags)
	return msg
}

func DecodeCreateCircuitV3Response(m *channel.Message) (*CreateCircuitV3Response, error) {
	circuitId, _ := m.GetStringHeader(CreateCircuitRespCircuitId)
	address, _ := m.GetStringHeader(CreateCircuitRespAddress)
	peerData, _, err := m.GetU32ToBytesMapHeader(CreateCircuitPeerDataHeader)
	if err != nil {
		return nil, fmt.Errorf("unable to get create circuit v3 response peer data (%w)", err)
	}

	tags, _, err := m.GetStringToStringMapHeader(CreateCircuitRespTagsHeader)
	if err != nil {
		return nil, fmt.Errorf("unable to get create circuit v3 response tags (%w)", err)
	}

	return &CreateCircuitV3Response{
		CircuitId: circuitId,
		Address:   address,
		PeerData:  peerData,
		Tags:      tags,
	}, nil
}

func DecodeCreateCircuitV3Request(m *channel.Message) (*CreateCircuitV3Request, error) {
	identityId, _ := m.GetStringHeader(CreateCircuitV3ReqIdentityIdHeader)
	if identityId == "" {
		return nil, errors.New("no identity id provided in create circuit v3 request")
	}

	serviceId, _ := m.GetStringHeader(CreateCircuitV3ReqServiceIdHeader)
	if serviceId == "" {
		return nil, errors.New("no service id provided in create circuit v3 request")
	}

	circuitId, _ := m.GetStringHeader(CreateCircuitV3ReqCircuitIdHeader)
	if circuitId == "" {
		return nil, errors.New("no circuit id provided in create circuit v3 request")
	}

	apiSessionToken, _ := m.GetStringHeader(CreateCircuitReqApiSessionTokenHeader)

	fingerprints, _, err := m.GetStringSliceHeader(CreateCircuitReqFingerprintsHeader)
	if err != nil {
		return nil, fmt.Errorf("unable to get create circuit v3 request fingerprints (%w)", err)
	}

	terminatorInstanceId, _ := m.GetStringHeader(CreateCircuitReqTerminatorInstanceIdHeader)
	peerData, _, err := m.GetU32ToBytesMapHeader(CreateCircuitPeerDataHeader)
	if err != nil {
		return nil, fmt.Errorf("unable to get create circuit v3 request peer data (%w)", err)
	}

	return &CreateCircuitV3Request{
		IdentityId:           identityId,
		ServiceId:            serviceId,
		CircuitId:            circuitId,
		Fingerprints:         fingerprints,
		TerminatorInstanceId: terminatorInstanceId,
		PeerData:             peerData,
		ApiSessionToken:      apiSessionToken,
	}, nil
}
