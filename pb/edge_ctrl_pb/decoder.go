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

package edge_ctrl_pb

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/channel2"
	"strings"
)

type Decoder struct{}

const DECODER = "edge_ctrl"

func (d Decoder) Decode(msg *channel2.Message) ([]byte, bool) {
	switch msg.ContentType {
	case int32(ContentType_ServerHelloType):
		request := &ServerHello{}
		if err := proto.Unmarshal(msg.Body, request); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Server Hello")
			meta["version"] = request.Version
			var extraData []string
			for k, v := range request.Data {
				extraData = append(extraData, fmt.Sprintf("%v=%v", k, v))
			}
			meta["data"] = strings.Join(extraData, ",")
			return meta.MarshalResult()
		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_ClientHelloType):
		request := &ClientHello{}
		if err := proto.Unmarshal(msg.Body, request); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Client Hello")
			meta["version"] = request.Version
			meta["hostname"] = request.Hostname
			meta["protocols"] = strings.Join(request.Protocols, ",")
			meta["protocolPorts"] = strings.Join(request.ProtocolPorts, ",")
			var data []string
			for k, v := range request.Data {
				data = append(data, fmt.Sprintf("%v=%v", k, v))
			}
			meta["data"] = strings.Join(data, ",")
			return meta.MarshalResult()
		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_ErrorType):
		meta := channel2.NewTraceMessageDecode(DECODER, "Edge Error")
		meta["error"] = string(msg.Body)
		return meta.MarshalResult()

	case int32(ContentType_SessionRemovedType):
		request := &SessionRemoved{}
		if err := proto.Unmarshal(msg.Body, request); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Session Removed")
			meta["ids"] = strings.Join(request.Ids, ",")
			meta["tokens"] = strings.Join(request.Tokens, ",")
			return meta.MarshalResult()
		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_CreateCircuitRequestType):
		request := &CreateCircuitRequest{}
		if err := proto.Unmarshal(msg.Body, request); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Create Circuit")
			meta["sessionToken"] = request.SessionToken
			meta["terminatorIdentity"] = request.TerminatorIdentity
			meta["fingerprints"] = strings.Join(request.Fingerprints, ",")
			return meta.MarshalResult()
		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}

	case int32(ContentType_CreateCircuitResponseType):
		request := &CreateCircuitResponse{}
		if err := proto.Unmarshal(msg.Body, request); err == nil {
			meta := channel2.NewTraceMessageDecode(DECODER, "Create Circuit Response")
			meta["circuitId"] = request.CircuitId
			meta["address"] = request.Address
			return meta.MarshalResult()
		} else {
			pfxlog.Logger().Errorf("unexpected error (%s)", err)
			return nil, true
		}
	}

	return nil, false
}
