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

package xgress

import (
	"encoding/json"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/ctrl_msg"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
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

func ReceiveRequest(peer transport.Conn) (*Request, error) {
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

func ReceiveResponse(peer transport.Conn) (*Response, error) {
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
	CtrlId      string
	CircuitId   *identity.TokenId
	Address     Address
	ResponseMsg *channel.Message
}

var circuitError = errors.New("error connecting circuit")

type networkControllers interface {
	AnyCtrlChannel() channel.Channel
	GetCtrlChannel(ctrlId string) channel.Channel
	DefaultRequestTimeout() time.Duration
	ForEach(f func(ctrlId string, ch channel.Channel))
}

func GetCircuit(ctrl networkControllers, ingressId string, service string, timeout time.Duration, peerData map[uint32][]byte) (*CircuitInfo, error) {
	ch := ctrl.AnyCtrlChannel()
	if ch == nil {
		return nil, errors.New("ctrl not ready")
	}

	log := pfxlog.Logger()
	circuitRequest := &ctrl_pb.CircuitRequest{
		IngressId: ingressId,
		Service:   service,
		PeerData:  peerData,
	}
	reply, err := protobufs.MarshalTyped(circuitRequest).WithTimeout(timeout).SendForReply(ch)
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

		log.WithField("circuitId", circuitId.Token).Debug("created new circuit")
		return &CircuitInfo{
			CtrlId:      ch.Id(),
			CircuitId:   circuitId,
			Address:     Address(address),
			ResponseMsg: reply,
		}, nil

	} else if reply.ContentType == ctrl_msg.CircuitFailedType {
		errMsg := string(reply.Body)
		log.WithError(err).Error("failure creating circuit")
		return nil, errors.New(errMsg)

	} else {
		log.Errorf("unexpected controller response to circuit request, response content type [%v]", reply.ContentType)
		return nil, circuitError
	}

}

func CreateCircuit(ctrl networkControllers, peer Connection, request *Request, bindHandler BindHandler, options *Options) *Response {
	circuitInfo, err := GetCircuit(ctrl, request.Id, request.ServiceId, options.GetCircuitTimeout, nil)
	if err != nil {
		return &Response{Success: false, Message: err.Error()}
	}

	x := NewXgress(circuitInfo.CircuitId.Token, circuitInfo.CtrlId, circuitInfo.Address, peer, Initiator, options, map[string]string{
		"serviceId": request.ServiceId,
	})
	bindHandler.HandleXgressBind(x)
	x.Start()

	return &Response{Success: true, CircuitId: circuitInfo.CircuitId.Token}
}

func RemoveTerminator(ctrls networkControllers, terminatorId string) error {
	log := pfxlog.Logger()
	request := &ctrl_pb.RemoveTerminatorRequest{
		TerminatorId: terminatorId,
	}
	responseMsg, err := protobufs.MarshalTyped(request).WithTimeout(ctrls.DefaultRequestTimeout()).SendForReply(ctrls.AnyCtrlChannel())
	if err != nil {
		log.WithError(err).Errorf("failed to send RemoveTerminatorRequest message")
		return err
	}

	if responseMsg != nil && responseMsg.ContentType == channel.ContentTypeResultType {
		result := channel.UnmarshalResult(responseMsg)
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
