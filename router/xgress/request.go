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

package xgress

import (
	"encoding/json"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/transport"
	"github.com/pkg/errors"
	"io"
	"time"
)

type Request struct {
	Id        string `json:"id"`
	ServiceId string `json:"svcId"`
}

func RequestFromJSON(payload []byte) (*Request, error) {
	request := &Request{}
	err := json.Unmarshal(payload, request)
	if err != nil {
		return nil, err
	}
	return request, nil
}

func (r *Request) ToJSON() ([]byte, error) {
	bytes, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

type Response struct {
	Success   bool   `json:"scc"`
	Message   string `json:"msg"`
	SessionId string `json:"sessionId"`
}

func ResponseFromJSON(payload []byte) (*Response, error) {
	response := &Response{}
	err := json.Unmarshal(payload, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (r *Response) ToJSON() ([]byte, error) {
	bytes, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func SendRequest(request *Request, peer io.Writer) error {
	requestJSON, err := request.ToJSON()
	if err != nil {
		return err
	}
	_, err = peer.Write(append(requestJSON, []byte{'\n'}...))
	if err != nil {
		return err
	}
	return nil
}

func ReceiveRequest(peer transport.Connection) (*Request, error) {
	line, err := ReadUntilNewline(peer)
	if err != nil {
		return nil, err
	}

	request, err := RequestFromJSON(line)
	if err != nil {
		return nil, err
	}

	return request, nil
}

func SendResponse(response *Response, peer io.Writer) error {
	responseJSON, err := response.ToJSON()
	if err != nil {
		return err
	}
	_, err = peer.Write(append(responseJSON, []byte{'\n'}...))
	if err != nil {
		return err
	}
	return nil
}

func ReceiveResponse(peer transport.Connection) (*Response, error) {
	line, err := ReadUntilNewline(peer)
	if err != nil {
		return nil, err
	}

	response, err := ResponseFromJSON(line)
	if err != nil {
		return nil, err
	}

	return response, nil
}

type SessionInfo struct {
	SessionId   *identity.TokenId
	Address     Address
	ResponseMsg *channel2.Message
	ctrl        CtrlChannel
}

var sessionError = errors.New("error connecting session")

func GetSession(ctrl CtrlChannel, ingressId string, serviceId string, timeout time.Duration, peerData map[uint32][]byte) (*SessionInfo, error) {
	log := pfxlog.Logger()
	sessionRequest := &ctrl_pb.SessionRequest{
		IngressId: ingressId,
		ServiceId: serviceId,
		PeerData:  peerData,
	}
	bytes, err := proto.Marshal(sessionRequest)
	if err != nil {
		log.Errorf("failed to marshal SessionRequest message (%v)", err)
		return nil, sessionError
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_SessionRequestType), bytes)
	reply, err := ctrl.Channel().SendAndWaitWithTimeout(msg, timeout)
	if err != nil {
		log.Errorf("failed to send SessionRequest message (%v)", err)
		return nil, sessionError
	}

	if reply.ContentType == ctrl_msg.SessionSuccessType {
		var address string

		sessionId := &identity.TokenId{Token: string(reply.Body)}
		sessionId.Data = make(map[uint32][]byte)
		for k, v := range reply.Headers {
			if k == ctrl_msg.SessionSuccessAddressHeader {
				address = string(v)
			} else {
				sessionId.Data[uint32(k)] = v
			}
		}

		log.Debugf("created new session [s/%s]", sessionId.Token)
		return &SessionInfo{
			SessionId:   sessionId,
			Address:     Address(address),
			ResponseMsg: reply,
			ctrl:        ctrl}, nil

	} else if reply.ContentType == ctrl_msg.SessionFailedType {
		errMsg := string(reply.Body)
		log.Errorf("failure creating session (%v)", errMsg)
		return nil, errors.New(errMsg)

	} else {
		log.Errorf("unexpected controller response, ContentType [%v]", msg.ContentType)
		return nil, sessionError
	}

}

func CreateSession(ctrl CtrlChannel, peer Connection, request *Request, bindHandler BindHandler, options *Options) *Response {
	sessionInfo, err := GetSession(ctrl, request.Id, request.ServiceId, options.GetSessionTimeout, nil)
	if err != nil {
		return &Response{Success: false, Message: err.Error()}
	}

	x := NewXgress(sessionInfo.SessionId, sessionInfo.Address, peer, Initiator, options)
	bindHandler.HandleXgressBind(x)
	x.Start()

	return &Response{Success: true, SessionId: sessionInfo.SessionId.Token}
}

func AddTerminator(ctrl CtrlChannel, serviceId, binding, address, identity string, identitySecret []byte, peerData map[uint32][]byte, staticCost uint16, precedence ctrl_pb.TerminatorPrecedence) (string, error) {
	log := pfxlog.Logger()
	request := &ctrl_pb.CreateTerminatorRequest{
		ServiceId:      serviceId,
		Binding:        binding,
		Address:        address,
		Identity:       identity,
		IdentitySecret: identitySecret,
		PeerData:       peerData,
		Cost:           uint32(staticCost),
		Precedence:     precedence,
	}
	bytes, err := proto.Marshal(request)
	if err != nil {
		log.Errorf("failed to marshal CreateTerminatorRequest message: (%v)", err)
		return "", sessionError
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_CreateTerminatorRequestType), bytes)
	responseMesg, err := ctrl.Channel().SendAndWaitWithTimeout(msg, ctrl.DefaultRequestTimeout())
	if err != nil {
		log.Errorf("failed to send CreateTerminatorRequest message (%v)", err)
		return "", sessionError
	}

	if responseMesg != nil && responseMesg.ContentType == channel2.ContentTypeResultType {
		result := channel2.UnmarshalResult(responseMesg)
		if result.Success {
			terminatorId := result.Message
			log.Debugf("successfully added service terminator [t/%s] for service [S/%v]", terminatorId, serviceId)
			return terminatorId, nil
		}
		log.Errorf("authentication failure: (%v)", result.Message)
		return "", errors.New(result.Message)
	} else {
		log.Errorf("unexpected controller response, ContentType [%v]", responseMesg.ContentType)
		return "", sessionError
	}
}

func RemoveTerminator(ctrl CtrlChannel, terminatorId string) error {
	log := pfxlog.Logger()
	request := &ctrl_pb.RemoveTerminatorRequest{
		TerminatorId: terminatorId,
	}
	bytes, err := proto.Marshal(request)
	if err != nil {
		log.Errorf("failed to marshal RemoveTerminatorRequest message (%v)", err)
		return sessionError
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_RemoveTerminatorRequestType), bytes)
	responseMsg, err := ctrl.Channel().SendAndWaitWithTimeout(msg, ctrl.DefaultRequestTimeout())
	if err != nil {
		log.Errorf("failed to send RemoveTerminatorRequest message (%v)", err)
		return sessionError
	}

	if responseMsg != nil && responseMsg.ContentType == channel2.ContentTypeResultType {
		result := channel2.UnmarshalResult(responseMsg)
		if result.Success {
			log.Debugf("successfully removed service terminator [s/%s]", terminatorId)
			return nil
		}
		log.Errorf("failure removing service terminator (%v)", result.Message)
		return errors.New(result.Message)
	} else {
		log.Errorf("unexpected controller response, ContentType [%v]", responseMsg.ContentType)
		return sessionError
	}
}

func UpdateTerminator(ctrl CtrlChannel, terminatorId string, staticCost *uint16, precedence *ctrl_pb.TerminatorPrecedence) error {
	log := pfxlog.Logger()
	request := &ctrl_pb.UpdateTerminatorRequest{
		TerminatorId:     terminatorId,
		UpdateCost:       staticCost != nil,
		UpdatePrecedence: precedence != nil,
	}
	if staticCost != nil {
		request.Cost = uint32(*staticCost)
	}
	if precedence != nil {
		request.Precedence = *precedence
	}
	bytes, err := proto.Marshal(request)
	if err != nil {
		log.Errorf("failed to marshal UpdateTerminatorRequest message (%v)", err)
		return sessionError
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_UpdateTerminatorRequestType), bytes)
	responseMsg, err := ctrl.Channel().SendAndWaitWithTimeout(msg, ctrl.DefaultRequestTimeout())
	if err != nil {
		log.Errorf("failed to send UpdateTerminatorRequest message (%v)", err)
		return sessionError
	}

	if responseMsg != nil && responseMsg.ContentType == channel2.ContentTypeResultType {
		result := channel2.UnmarshalResult(responseMsg)
		if result.Success {
			log.Debugf("successfully updated service terminator [t/%s]", terminatorId)
			return nil
		}
		log.Errorf("failure updating service terminator (%v)", result.Message)
		return errors.New(result.Message)
	} else {
		log.Errorf("unexpected controller response, ContentType [%v]", responseMsg.ContentType)
		return sessionError
	}
}
