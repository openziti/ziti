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
		log.Debugf("assembling with [%d] routers", network.Routers.connectedCount())

		missingLinks, err := network.linkController.missingLinks(network.Routers.allConnected(), network.options.PendingLinkTimeout)
		if err == nil {
			for _, missingLink := range missingLinks {
				network.linkController.add(missingLink)

				dial := &ctrl_pb.Dial{
					LinkId:   missingLink.Id,
					Address:  missingLink.Dst.AdvertisedListener,
					RouterId: missingLink.Dst.Id,
				}

				if err := protobufs.MarshalTyped(dial).Send(missingLink.Src.Control); err != nil {
					log.WithError(err).Error("unexpected error sending dial")
				}
			}

		} else {
			log.WithField("err", err).Error("missing link enumeration failed")
		}
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
		log.WithField("linkId", lr.Id).Info("removing failed link", lr.Id)
		network.linkController.remove(lr)
	}
}
