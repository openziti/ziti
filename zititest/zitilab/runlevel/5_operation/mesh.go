/*
	Copyright 2019 NetFoundry Inc.

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

package zitilib_runlevel_5_operation

import (
	"fmt"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/identity/dotziti"
	"github.com/openziti/transport/v2"
	"github.com/sirupsen/logrus"
	"time"
)

func Mesh(closer <-chan struct{}) model.OperatingStage {
	return &mesh{closer: closer}
}

func (mesh *mesh) Operate(run model.Run) error {
	if endpoint, id, err := dotziti.LoadIdentity(model.ActiveInstanceId()); err == nil {
		if address, err := transport.ParseAddress(endpoint); err == nil {
			dialer := channel.NewClassicDialer(id, address, nil)
			if ch, err := channel.NewChannel("mesh", dialer, nil, nil); err == nil {
				mesh.ch = ch
			} else {
				return fmt.Errorf("error connecting mesh channel (%w)", err)
			}
		} else {
			return fmt.Errorf("invalid endpoint address (%w)", err)
		}
	} else {
		return fmt.Errorf("unable to load 'fablab' identity (%w)", err)
	}

	mesh.m = run.GetModel()

	go mesh.runMesh()

	return nil
}

func (mesh *mesh) runMesh() {
	logrus.Infof("starting")
	defer logrus.Infof("exiting")

	for {
		select {
		case <-time.After(15 * time.Second):
			if err := mesh.interrogate(); err != nil {
				logrus.Errorf("error querying mesh state (%v)", err)
			}

		case <-mesh.closer:
			_ = mesh.ch.Close()
			return
		}
	}
}

// TODO: Update to use REST client
func (mesh *mesh) interrogate() error {
	/*
		var err error
		var body []byte
		var listRoutersWaitCh chan *channel.Message
		var listLinksWaitCh chan *channel.Message

		listRoutersRequest := &mgmt_pb.ListRoutersRequest{}
		body, err = proto.Marshal(listRoutersRequest)
		if err != nil {
			return fmt.Errorf("error marshaling router list request (%w)", err)
		}
		requestMsg := channel.NewMessage(int32(mgmt_pb.ContentType_ListRoutersRequestType), body)
		listRoutersWaitCh, err = mesh.ch.SendAndWait(requestMsg)
		if err != nil {
			return fmt.Errorf("error queuing router list request (%w)", err)
		}

		listLinksRequest := &mgmt_pb.ListLinksRequest{}
		body, err = proto.Marshal(listLinksRequest)
		if err != nil {
			return fmt.Errorf("error marshaling links list request (%w)", err)
		}
		requestMsg = channel.NewMessage(int32(mgmt_pb.ContentType_ListLinksRequestType), body)
		listLinksWaitCh, err = mesh.ch.SendAndWait(requestMsg)
		if err != nil {
			return fmt.Errorf("error queuing link list request (%w)", err)
		}

		summary := model.ZitiFabricMeshSummary{TimestampMs: info.NowInMilliseconds()}
		select {
		case listRoutersResponseMsg := <-listRoutersWaitCh:
			response := &mgmt_pb.ListRoutersResponse{}
			err := proto.Unmarshal(listRoutersResponseMsg.Body, response)
			if err != nil {
				return fmt.Errorf("error unmarshaling router list response (%w)", err)
			}
			for _, r := range response.Routers {
				if r.Connected {
					summary.RouterIds = append(summary.RouterIds, r.Id)
				}
			}
		}
		select {
		case listLinksResponseMsg := <-listLinksWaitCh:
			response := &mgmt_pb.ListLinksResponse{}
			err := proto.Unmarshal(listLinksResponseMsg.Body, response)
			if err != nil {
				return fmt.Errorf("error unmarshaling links list response (%w)", err)
			}
			for _, l := range response.Links {
				summary.Links = append(summary.Links, model.ZitiFabricLinkSummary{
					LinkId:      l.Id,
					State:       l.State,
					SrcRouterId: l.Src,
					SrcLatency:  float64(l.SrcLatency),
					DstRouterId: l.Dst,
					DstLatency:  float64(l.DstLatency),
				})
			}
		}

		if mesh.m.Data == nil {
			mesh.m.Data = make(map[string]interface{})
		}
		if _, found := mesh.m.Data["fabric_mesh"]; !found {
			mesh.m.Data["fabric_mesh"] = make([]model.ZitiFabricMeshSummary, 0)
		}

		summaries := mesh.m.Data["fabric_mesh"].([]model.ZitiFabricMeshSummary)
		summaries = append(summaries, summary)
		mesh.m.Data["fabric_mesh"] = summaries

		logrus.Infof("</=")
	*/

	return nil
}

type mesh struct {
	ch     channel.Channel
	m      *model.Model
	closer <-chan struct{}
}
