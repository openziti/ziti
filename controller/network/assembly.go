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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/model"
	"time"
)

func (network *Network) assemble() {
	if !network.options.EnableLegacyLinkMgmt {
		return
	}

	log := pfxlog.Logger()

	if network.Router.ConnectedCount() > 1 {
		log.Tracef("assembling with [%d] routers", network.Router.ConnectedCount())

		missingLinks, err := network.Link.MissingLinks(network.Router.AllConnected(), network.options.PendingLinkTimeout)
		if err == nil {
			for _, missingLink := range missingLinks {
				network.Link.Add(missingLink)

				dial := &ctrl_pb.Dial{
					LinkId:       missingLink.Id,
					Address:      missingLink.DialAddress,
					RouterId:     missingLink.DstId,
					LinkProtocol: missingLink.Protocol,
				}

				if versionInfo := missingLink.GetDest().VersionInfo; versionInfo != nil {
					dial.RouterVersion = missingLink.GetDest().VersionInfo.Version
				}

				if err = protobufs.MarshalTyped(dial).Send(missingLink.Src.Control); err != nil {
					log.WithField("linkId", missingLink.Id).
						WithField("srcRouterId", missingLink.Src.Id).
						WithField("dstRouterId", missingLink.DstId).
						WithError(err).Error("unexpected error sending dial")
				} else {
					log.WithField("linkId", missingLink.Id).
						WithField("srcRouterId", missingLink.Src.Id).
						WithField("dstRouterId", missingLink.DstId).
						Info("sending link dial")
					network.NotifyLinkEvent(missingLink, event.LinkDialed)
				}
			}
		} else {
			log.WithField("err", err).Error("missing link enumeration failed")
		}

		network.Link.ClearExpiredPending(network.options.PendingLinkTimeout)
	}
}

func (network *Network) NotifyLinkEvent(link *model.Link, eventType event.LinkEventType) {
	linkEvent := &event.LinkEvent{
		Namespace:   event.LinkEventsNs,
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
	network.eventDispatcher.AcceptLinkEvent(linkEvent)
}

func (network *Network) NotifyLinkConnected(link *model.Link, msg *ctrl_pb.LinkConnected) {
	linkEvent := &event.LinkEvent{
		Namespace:   event.LinkEventsNs,
		EventType:   event.LinkConnected,
		EventSrcId:  network.GetAppId(),
		Timestamp:   time.Now(),
		LinkId:      link.Id,
		SrcRouterId: link.Src.Id,
		DstRouterId: link.DstId,
		Protocol:    link.Protocol,
		Cost:        link.GetStaticCost(),
		DialAddress: link.DialAddress,
	}

	for _, conn := range msg.Conns {
		linkEvent.Connections = append(linkEvent.Connections, &event.LinkConnection{
			Id:         conn.Id,
			LocalAddr:  conn.LocalAddr,
			RemoteAddr: conn.RemoteAddr,
		})
	}

	network.eventDispatcher.AcceptLinkEvent(linkEvent)
}

func (network *Network) NotifyLinkIdEvent(linkId string, eventType event.LinkEventType) {
	linkEvent := &event.LinkEvent{
		Namespace:  event.LinkEventsNs,
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
