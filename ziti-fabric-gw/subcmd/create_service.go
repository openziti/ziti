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

package subcmd

import (
	"encoding/json"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/foundation/channel2"
	"net/http"
)

func handleCreateService(w http.ResponseWriter, req *http.Request) {
	defer func() { _ = req.Body.Close() }()

	decoder := json.NewDecoder(req.Body)
	var svc service
	err := decoder.Decode(&svc)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(err.Error()); err != nil {
			pfxlog.Logger().Errorf("error encoding json (%s)", err)
		}
		return
	}

	request := &mgmt_pb.CreateServiceRequest{
		Service: &mgmt_pb.Service{
			Id: svc.Id,
			Terminators: []*mgmt_pb.Terminator{{
				RouterId: svc.Egress,
				Binding:  svc.Binding,
				Address:  svc.EndpointAddress,
			}},
		},
	}
	body, err := proto.Marshal(request)
	if err != nil {
		panic(err)
	}
	requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_CreateServiceRequestType), body)
	waitCh, err := mgmtCh.SendAndWait(requestMsg)
	if err == nil {
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == channel2.ContentTypeResultType {
				result := channel2.UnmarshalResult(responseMsg)
				if result.Success {
					if err := json.NewEncoder(w).Encode(&data{Data: make([]interface{}, 0)}); err != nil {
						pfxlog.Logger().Errorf("error encoding json (%s)", err)
					}
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusInternalServerError)
					if err := json.NewEncoder(w).Encode(result.Message); err != nil {
						pfxlog.Logger().Errorf("error encoding json (%s)", err)
					}
				}
			}
		}
	} else {
		pfxlog.Logger().Errorf("unexpected error (%s)", err)
	}
}
