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

package network

import (
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/model"
)

func (network *Network) NotifyLinkEvent(link *model.Link, eventType event.LinkEventType) {
	linkEvent := &event.LinkEvent{
		Namespace:   event.LinkEventNS,
		EventType:   eventType,
		EventSrcId:  network.GetAppId(),
		Timestamp:   time.Now(),
		LinkId:      link.Id,
		SrcRouterId: link.Src.Id,
		DstRouterId: link.DstId,
		Protocol:    link.Protocol,
		Cost:        link.GetStaticCost(),
		DialAddress: link.DialAddress,
	}

	if connState := link.GetConnsState(); connState != nil {
		for _, c := range connState.GetConns() {
			linkEvent.Connections = append(linkEvent.Connections, &event.LinkConnection{
				Id:         c.Type,
				LocalAddr:  c.LocalAddr,
				RemoteAddr: c.RemoteAddr,
			})
		}
	}
	network.eventDispatcher.AcceptLinkEvent(linkEvent)
}

func (network *Network) NotifyLinkIdEvent(linkId string, eventType event.LinkEventType) {
	linkEvent := &event.LinkEvent{
		Namespace:  event.LinkEventNS,
		EventType:  eventType,
		EventSrcId: network.GetAppId(),
		Timestamp:  time.Now(),
		LinkId:     linkId,
	}
	network.eventDispatcher.AcceptLinkEvent(linkEvent)
}

func (network *Network) clean() {
	log := pfxlog.Logger()

	failedLinks := network.Link.LinksInMode(model.Failed)
	duplicateLinks := network.Link.LinksInMode(model.Duplicate)
	failedLinks = append(failedLinks, duplicateLinks...)

	now := info.NowInMilliseconds()
	var lRemove []*model.Link
	for _, l := range failedLinks {
		if now-l.CurrentState().Timestamp >= 30000 {
			lRemove = append(lRemove, l)
		}
	}
	for _, lr := range lRemove {
		log.WithField("linkId", lr.Id).Info("removing failed link")
		network.Link.Remove(lr)
	}
}
