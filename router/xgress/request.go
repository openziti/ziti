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
	CircuitId string `json:"circuitId"`
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

type CircuitInfo struct {
	CircuitId   *identity.TokenId
	Address     Address
	ResponseMsg *channel2.Message
	ctrl        CtrlChannel
}

var circuitError = errors.New("error connecting circuit")

func GetCircuit(ctrl CtrlChannel, ingressId string, serviceId string, timeout time.Duration, peerData map[uint32][]byte) (*CircuitInfo, error) {
	if ctrl == nil || ctrl.Channel() == channel2.Channel(nil) {
		return nil, errors.New("ctrl not ready")
	}

	log := pfxlog.Logger()
	circuitRequest := &ctrl_pb.CircuitRequest{
		IngressId: ingressId,
		ServiceId: serviceId,
		PeerData:  peerData,
	}
	bytes, err := proto.Marshal(circuitRequest)
	if err != nil {
		log.Errorf("failed to marshal CircuitRequest message (%v)", err)
		return nil, circuitError
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_CircuitRequestType), bytes)
	reply, err := ctrl.Channel().SendAndWaitWithTimeout(msg, timeout)
	if err != nil {
		log.Errorf("failed to send CircuitRequest message (%v)", err)
		return nil, circuitError
	}

	if reply.ContentType == ctrl_msg.CircuitSuccessType {
		var address string

		circuitId := &identity.TokenId{Token: string(reply.Body)}
		circuitId.Data = make(map[uint32][]byte)
		for k, v := range reply.Headers {
			if k == ctrl_msg.CircuitSuccessAddressHeader {
				address = string(v)
			} else {
				circuitId.Data[uint32(k)] = v
			}
		}

		log.Debugf("created new circuit [s/%s]", circuitId.Token)
		return &CircuitInfo{
			CircuitId:   circuitId,
			Address:     Address(address),
			ResponseMsg: reply,
			ctrl:        ctrl}, nil

	} else if reply.ContentType == ctrl_msg.CircuitFailedType {
		errMsg := string(reply.Body)
		log.Errorf("failure creating circuit (%v)", errMsg)
		return nil, errors.New(errMsg)

	} else {
		log.Errorf("unexpected controller response, ContentType [%v]", msg.ContentType)
		return nil, circuitError
	}

}

func CreateCircuit(ctrl CtrlChannel, peer Connection, request *Request, bindHandler BindHandler, options *Options) *Response {
	circuitInfo, err := GetCircuit(ctrl, request.Id, request.ServiceId, options.GetCircuitTimeout, nil)
	if err != nil {
		return &Response{Success: false, Message: err.Error()}
	}

	x := NewXgress(circuitInfo.CircuitId, circuitInfo.Address, peer, Initiator, options)
	bindHandler.HandleXgressBind(x)
	x.Start()

	return &Response{Success: true, CircuitId: circuitInfo.CircuitId.Token}
}

func RemoveTerminator(ctrl CtrlChannel, terminatorId string) error {
	log := pfxlog.Logger()
	request := &ctrl_pb.RemoveTerminatorRequest{
		TerminatorId: terminatorId,
	}
	bytes, err := proto.Marshal(request)
	if err != nil {
		log.WithError(err).Errorf("failed to marshal RemoveTerminatorRequest message")
		return err
	}

	msg := channel2.NewMessage(int32(ctrl_pb.ContentType_RemoveTerminatorRequestType), bytes)
	responseMsg, err := ctrl.Channel().SendAndWaitWithTimeout(msg, ctrl.DefaultRequestTimeout())
	if err != nil {
		log.WithError(err).Errorf("failed to send RemoveTerminatorRequest message")
		return err
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
		return errors.Errorf("unexpected controller response, ContentType [%v]", responseMsg.ContentType)
	}
}
