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
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"encoding/json"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"net/http"
	"time"
)

func handleListLinks(w http.ResponseWriter, _ *http.Request) {
	request := &mgmt_pb.ListLinksRequest{}
	body, err := proto.Marshal(request)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		pfxlog.Logger().Errorf("error encoding json (%s)", err)
		return
	}

	requestMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_ListLinksRequestType), body)
	waitCh, err := mgmtCh.SendAndWait(requestMsg)
	if err == nil {
		select {
		case responseMsg := <-waitCh:
			if responseMsg.ContentType == int32(mgmt_pb.ContentType_ListLinksResponseType) {
				response := &mgmt_pb.ListLinksResponse{}
				err := proto.Unmarshal(responseMsg.Body, response)
				if err == nil {
					if err := json.NewEncoder(w).Encode(convertLinks(response.Links)); err != nil {
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

func convertLinks(in []*mgmt_pb.Link) interface{} {
	d := &data{}
	for _, l := range in {
		srcLatency := float64(l.SrcLatency) / 1000000000.0
		dstLatency := float64(l.DstLatency) / 1000000000.0
		d.Data = append(d.Data, &link{
			Id:         l.Id,
			Src:        l.Src,
			Dst:        l.Dst,
			State:      l.State,
			Down:       l.Down,
			Cost:       l.Cost,
			SrcLatency: srcLatency,
			DstLatency: dstLatency,
		})
	}
	return d
}
