/*
	Copyright 2019 NetFoundry, Inc.

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
	"github.com/netfoundry/ziti-fabric/ctrl_msg"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
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
	Success bool   `json:"scc"`
	Message string `json:"msg"`
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
	line, err := transport.ReadUntilNewline(peer)
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
	line, err := transport.ReadUntilNewline(peer)
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

func (si *SessionInfo) SendStartEgress() error {
	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_StartXgressType), nil)
	msg.ReplyTo(si.ResponseMsg)
	return si.ctrl.Channel().SendWithTimeout(msg, time.Second*5)
}

var authError = errors.New("unexpected failure while authenticating")

func GetSession(ctrl CtrlChannel, ingressId string, serviceId string) (*SessionInfo, error) {
	log := pfxlog.Logger()
	sessionRequest := &ctrl_pb.SessionRequest{
		IngressId: ingressId,
		ServiceId: serviceId,
	}
	bytes, err := proto.Marshal(sessionRequest)
	if err != nil {
		log.Errorf("failed to marshal SessionRequest message: (%v)", err)
		return nil, authError
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_SessionRequestType), bytes)
	reply, err := ctrl.Channel().SendAndWaitWithTimeout(msg, time.Second*5)
	if err != nil {
		log.Errorf("failed to send SessionRequest message: (%v)", err)
		return nil, authError
	}

	if reply.ContentType == ctrl_msg.ContentTypeSessionSuccessType {
		sessionId := &identity.TokenId{Token: string(reply.Body)}
		address := string(reply.Headers[ctrl_msg.SessionSuccessAddressHeader])
		log.Debugf("created new session [s/%s]", sessionId.Token)
		return &SessionInfo{
			SessionId:   sessionId,
			Address:     Address(address),
			ResponseMsg: reply,
			ctrl:        ctrl}, nil
	} else if reply.ContentType == ctrl_msg.ContentTypeSessionFailedType {
		errMsg := string(reply.Body)
		log.Errorf("authentication failure: (%v)", errMsg)
		return nil, errors.New(errMsg)
	} else {
		log.Errorf("unexpected controller response, ContentType: (%v)", msg.ContentType)
		return nil, authError
	}

}

func CreateSession(ctrl CtrlChannel, peer Connection, request *Request, bindHandler BindHandler, options *Options) *Response {
	sessionInfo, err := GetSession(ctrl, request.Id, request.ServiceId)
	if err != nil {
		return &Response{Success: false, Message: err.Error()}
	}

	x := NewXgress(sessionInfo.SessionId, sessionInfo.Address, peer, Initiator, options)
	bindHandler.HandleXgressBind(sessionInfo.SessionId, sessionInfo.Address, Initiator, x)
	x.Start()

	if err = sessionInfo.SendStartEgress(); err != nil {
		return &Response{Success: false, Message: err.Error()}
	}
	return &Response{Success: true}
}

func BindService(ctrl CtrlChannel, token string, serviceId string) error {
	log := pfxlog.Logger()
	hostRequest := &ctrl_pb.BindRequest{
		BindType:  ctrl_pb.BindType_Bind,
		Token:     token,
		ServiceId: serviceId,
	}
	bytes, err := proto.Marshal(hostRequest)
	if err != nil {
		log.Errorf("failed to marshal BindRequest message: (%v)", err)
		return authError
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_BindRequestType), bytes)
	waitCh, err := ctrl.Channel().SendAndWait(msg)
	if err != nil {
		log.Errorf("failed to send BindRequest message: (%v)", err)
		return authError
	}

	select {
	case msg := <-waitCh:
		if msg != nil && msg.ContentType == int32(ctrl_pb.ContentType_BindResponseType) {
			bindResponse := &ctrl_pb.BindResponse{}
			err := proto.Unmarshal(msg.Body, bindResponse)
			if err != nil {
				log.Errorf("failed to send un-marshall BindResponse message: (%v)", err)
				return authError
			}
			if bindResponse.Success {
				log.Debugf("successfully bound session [s/%s]", token)
				return nil
			}
			log.Errorf("authentication failure: (%v)", bindResponse.Message)
			return errors.New(bindResponse.Message)
		} else {
			log.Errorf("unexpected controller response, ContentType: (%v)", msg.ContentType)
			return authError
		}

	case <-time.After(5 * time.Second):
		return errors.New("timeout while binding")
	}
}

func UnbindService(ctrl CtrlChannel, token string, serviceId string) error {
	log := pfxlog.Logger()
	hostRequest := &ctrl_pb.BindRequest{
		BindType:  ctrl_pb.BindType_Unbind,
		Token:     token,
		ServiceId: serviceId,
	}
	bytes, err := proto.Marshal(hostRequest)
	if err != nil {
		log.Errorf("failed to marshal BindRequest message: (%v)", err)
		return authError
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_BindRequestType), bytes)
	waitCh, err := ctrl.Channel().SendAndWait(msg)
	if err != nil {
		log.Errorf("failed to send HostRequest message: (%v)", err)
		return authError
	}

	select {
	case msg := <-waitCh:
		if msg != nil && msg.ContentType == int32(ctrl_pb.ContentType_BindResponseType) {
			bindResponse := &ctrl_pb.BindResponse{}
			err := proto.Unmarshal(msg.Body, bindResponse)
			if err != nil {
				log.Errorf("failed to send un-marshall BindResponse message: (%v)", err)
				return authError
			}
			if bindResponse.Success {
				log.Debugf("successfully unbound session [s/%s]", token)
				return nil
			}
			log.Errorf("authentication failure: (%v)", bindResponse.Message)
			return errors.New(bindResponse.Message)
		} else {
			log.Errorf("unexpected controller response, ContentType: (%v)", msg.ContentType)
			return authError
		}

	case <-time.After(5 * time.Second):
		return errors.New("timeout while unbinding")
	}
}
