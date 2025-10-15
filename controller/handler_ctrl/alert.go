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

package handler_ctrl

import (
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	alertConst "github.com/openziti/ziti/common/alert"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/network"
	"google.golang.org/protobuf/proto"
)

type alertHandler struct {
	r          *model.Router
	n          *network.Network
	dispatcher event.Dispatcher
}

func newAlertHandler(r *model.Router, n *network.Network) *alertHandler {
	return &alertHandler{
		r:          r,
		n:          n,
		dispatcher: n.GetEventDispatcher(),
	}
}

func (self *alertHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_AlertsType)
}

func (self *alertHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	alertsMsg := &ctrl_pb.Alerts{}
	if err := proto.Unmarshal(msg.Body, alertsMsg); err == nil {
		for _, alertMsg := range alertsMsg.Alerts {
			alert := &event.AlertEvent{
				Namespace:       event.AlertEventNS,
				EventSrcId:      self.n.GetAppId(),
				Timestamp:       time.UnixMilli(alertMsg.Timestamp),
				AlertSourceType: alertMsg.SourceType,
				AlertSourceId:   alertMsg.SourceId,
				Severity:        alertMsg.Severity,
				Message:         alertMsg.Message,
				Details:         alertMsg.Details,
				RelatedEntities: alertMsg.RelatedEntities,
			}

			if alert.AlertSourceType == alertConst.EntityTypeSelf {
				alert.AlertSourceType = event.AlertSourceTypeRouter
				alert.AlertSourceId = self.r.Id
			}

			if selfType, ok := alert.RelatedEntities[alertConst.EntityTypeSelf]; ok {
				alert.RelatedEntities[self.n.GetStores().Router.GetSingularEntityType()] = self.r.Id
				if selfType == alertConst.EntityTypeSelfErt {
					alert.RelatedEntities[self.n.GetStores().Identity.GetSingularEntityType()] = self.r.Id
				}
				delete(alert.RelatedEntities, alertConst.EntityTypeSelf)
			}

			self.dispatcher.AcceptAlertEvent(alert)
		}
	} else {
		pfxlog.ContextLogger(ch.Label()).WithError(err).Error("unexpected error unmarshalling alert")
	}
}
