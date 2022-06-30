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
	"github.com/openziti/channel/protobufs"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/foundation/util/info"
)

func (network *Network) assemble() {
	log := pfxlog.Logger()

	if network.Routers.connectedCount() > 1 {
		log.Tracef("assembling with [%d] routers", network.Routers.connectedCount())

		missingLinks, err := network.linkController.missingLinks(network.Routers.allConnected(), network.options.PendingLinkTimeout)
		if err == nil {
			for _, missingLink := range missingLinks {
				network.linkController.add(missingLink)

				for _, listener := range missingLink.Dst.Listeners {
					if listener.Protocol() != missingLink.Protocol {
						continue
					}
					dial := &ctrl_pb.Dial{
						LinkId:       missingLink.Id,
						Address:      listener.AdvertiseAddress(),
						RouterId:     missingLink.Dst.Id,
						LinkProtocol: listener.Protocol(),
					}

					if versionInfo := missingLink.Dst.VersionInfo; versionInfo != nil {
						dial.RouterVersion = missingLink.Dst.VersionInfo.Version
					}

					if err = protobufs.MarshalTyped(dial).Send(missingLink.Src.Control); err != nil {
						log.WithError(err).Error("unexpected error sending dial")
					} else {
						log.WithField("linkId", dial.LinkId).
							WithField("srcRouterId", missingLink.Src.Id).
							WithField("dstRouterId", missingLink.Dst.Id).
							Info("sending link dial")
					}
				}
			}
		} else {
			log.WithField("err", err).Error("missing link enumeration failed")
		}

		network.linkController.clearExpiredPending(network.options.PendingLinkTimeout)
	}
}

func (network *Network) clean() {
	log := pfxlog.Logger()

	failedLinks := network.linkController.linksInMode(Failed)

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
