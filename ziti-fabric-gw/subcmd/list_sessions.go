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
	"time"
)

func handleListCircuits(w http.ResponseWriter, _ *http.Request) {
	request := &mgmt_pb.ListCircuitsRequest{}
	body, err := proto.Marshal(request)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		pfxlog.Logger().Errorf("error encoding json (%s)", err)
		return
	}

	requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListCircuitsRequestType), body)
	waitCh, err := mgmtCh.SendAndWait(requestMsg)
	if err == nil {
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListCircuitsResponseType) {
				response := &mgmt_pb.ListCircuitsResponse{}
				err := proto.Unmarshal(responseMsg.Body, response)
				if err == nil {
					if err := json.NewEncoder(w).Encode(convertCircuits(response.Circuits)); err != nil {
						pfxlog.Logger().Errorf("error encoding json (%s)", err)
					}

				} else {
					w.WriteHeader(http.StatusInternalServerError)
					if err := json.NewEncoder(w).Encode(err.Error()); err != nil {
						pfxlog.Logger().Errorf("error encoding json (%s)", err)
					}
				}
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				if err := json.NewEncoder(w).Encode("unexpected response"); err != nil {
					pfxlog.Logger().Errorf("error encoding json (%s)", err)
				}
			}

		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusInternalServerError)
			if err := json.NewEncoder(w).Encode("timeout"); err != nil {
				pfxlog.Logger().Errorf("error encoding json (%s)", err)
			}
		}
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(err.Error()); err != nil {
			pfxlog.Logger().Errorf("error encoding json (%s)", err)
		}
	}
}

func convertCircuits(in []*mgmt_pb.Circuit) interface{} {
	d := &data{}
	for _, s := range in {
		c := &path{}
		for _, p := range s.Path.Nodes {
			c.Path = append(c.Path, p)
		}
		for _, l := range s.Path.Links {
			c.Links = append(c.Links, l)
		}
		d.Data = append(d.Data, &circuit{
			Id:        s.Id,
			ClientId:  s.ClientId,
			ServiceId: s.ServiceId,
			Circuit:   c,
		})
	}
	return d
}
