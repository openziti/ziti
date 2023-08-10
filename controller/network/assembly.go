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
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fabric/controller/event"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/v2/info"
	"time"
)

func (network *Network) assemble() {
	if !network.options.EnableLegacyLinkMgmt {
		return
	}

	log := pfxlog.Logger()

	if network.Routers.connectedCount() > 1 {
		log.Tracef("assembling with [%d] routers", network.Routers.connectedCount())

		missingLinks, err := network.linkController.missingLinks(network.Routers.allConnected(), network.options.PendingLinkTimeout)
		if err == nil {
			for _, missingLink := range missingLinks {
				network.linkController.add(missingLink)

				dial := &ctrl_pb.Dial{
					LinkId:       missingLink.Id,
					Address:      missingLink.DialAddress,
					RouterId:     missingLink.Dst.Id,
					LinkProtocol: missingLink.Protocol,
				}

				if versionInfo := missingLink.Dst.VersionInfo; versionInfo != nil {
					dial.RouterVersion = missingLink.Dst.VersionInfo.Version
				}

				if err = protobufs.MarshalTyped(dial).Send(missingLink.Src.Control); err != nil {
					log.WithField("linkId", missingLink.Id).
						WithField("srcRouterId", missingLink.Src.Id).
						WithField("dstRouterId", missingLink.Dst.Id).
						WithError(err).Error("unexpected error sending dial")
				} else {
					log.WithField("linkId", missingLink.Id).
						WithField("srcRouterId", missingLink.Src.Id).
						WithField("dstRouterId", missingLink.Dst.Id).
						Info("sending link dial")
					network.NotifyLinkEvent(missingLink, event.LinkDialed)
				}
			}
		} else {
			log.WithField("err", err).Error("missing link enumeration failed")
		}

		network.linkController.clearExpiredPending(network.options.PendingLinkTimeout)
	}
}

func (network *Network) NotifyLinkEvent(link *Link, eventType event.LinkEventType) {
	linkEvent := &event.LinkEvent{
		Namespace:   event.LinkEventsNs,
		EventType:   eventType,
		Timestamp:   time.Now(),
		LinkId:      link.Id,
		SrcRouterId: link.Src.Id,
		DstRouterId: link.Dst.Id,
		Protocol:    link.Protocol,
		Cost:        link.GetStaticCost(),
		DialAddress: link.DialAddress,
	}
	network.eventDispatcher.AcceptLinkEvent(linkEvent)
}

func (network *Network) NotifyLinkConnected(link *Link, msg *ctrl_pb.LinkConnected) {
	linkEvent := &event.LinkEvent{
		Namespace:   event.LinkEventsNs,
		EventType:   event.LinkConnected,
		Timestamp:   time.Now(),
		LinkId:      link.Id,
		SrcRouterId: link.Src.Id,
		DstRouterId: link.Dst.Id,
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
		Namespace: event.LinkEventsNs,
		EventType: eventType,
		Timestamp: time.Now(),
		LinkId:    linkId,
	}
	network.eventDispatcher.AcceptLinkEvent(linkEvent)
}

func (network *Network) clean() {
	log := pfxlog.Logger()

	failedLinks := network.linkController.linksInMode(Failed)
	duplicateLinks := network.linkController.linksInMode(Duplicate)
	failedLinks = append(failedLinks, duplicateLinks...)

	now := info.NowInMilliseconds()
	lRemove := make(map[string]*Link)
	for _, l := range failedLinks {
		if now-l.CurrentState().Timestamp >= 30000 {
			lRemove[l.Id] = l
		}
	}
	for _, lr := range lRemove {
		log.WithField("linkId", lr.Id).Info("removing failed link")
		network.linkController.remove(lr)
	}
}
