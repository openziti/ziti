/*
	Copyright 2019 Netfoundry, Inc.

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
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"encoding/json"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"net/http"
	"time"
)

func handleListSessions(w http.ResponseWriter, _ *http.Request) {
	request := &mgmt_pb.ListSessionsRequest{}
	body, err := proto.Marshal(request)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		pfxlog.Logger().Errorf("error encoding json (%s)", err)
		return
	}

	requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListSessionsRequestType), body)
	waitCh, err := mgmtCh.SendAndWait(requestMsg)
	if err == nil {
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListSessionsResponseType) {
				response := &mgmt_pb.ListSessionsResponse{}
				err := proto.Unmarshal(responseMsg.Body, response)
				if err == nil {
					if err := json.NewEncoder(w).Encode(convertSessions(response.Sessions)); err != nil {
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

func convertSessions(in []*mgmt_pb.Session) interface{} {
	d := &data{}
	for _, s := range in {
		c := &circuit{}
		for _, p := range s.Circuit.Path {
			c.Path = append(c.Path, p)
		}
		for _, l := range s.Circuit.Links {
			c.Links = append(c.Links, l)
		}
		d.Data = append(d.Data, &session{
			Id:        s.Id,
			ClientId:  s.ClientId,
			ServiceId: s.ServiceId,
			Circuit:   c,
		})
	}
	return d
}
