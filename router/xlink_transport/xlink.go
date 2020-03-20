/*
	(c) Copyright NetFoundry, Inc.

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

package xlink_transport

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/netfoundry/ziti-fabric/metrics"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/router/handler_link"
	"github.com/netfoundry/ziti-fabric/router/xgress"
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
	log "github.com/sirupsen/logrus"
)

func (self *impl) Listen() error {
	self.listener = channel2.NewClassicListener(self.id, self.config.listener)
	if err := self.listener.Listen(); err != nil {
		return fmt.Errorf("error listening (%w)", err)
	}
	go handler_link.NewAccepter(self.id, self.ctrl, self.forwarder, self.listener, self.linkOptions, self.forwarderOptions)
	return nil
}

func (self *impl) Dial(addressString, linkId string) error {
	address, err := transport.ParseAddress(addressString)
	if err == nil {
		linkId := self.id.ShallowCloneWithNewToken(linkId)
		name := "l/" + linkId.Token

		dialer := channel2.NewClassicDialer(linkId, address, nil)
		ch, err := channel2.NewChannel(name, dialer, self.linkOptions)
		if err == nil {
			link := forwarder.NewLink(linkId, ch)
			if err := link.Channel.Bind(handler_link.NewBindHandler(self.id, link, self.ctrl, self.forwarder)); err == nil {
				self.forwarder.RegisterLink(link)

				go metrics.ProbeLatency(
					link.Channel,
					self.metricsRegistry.Histogram("link."+linkId.Token+".latency"),
					self.forwarderOptions.LatencyProbeInterval,
				)

				linkMsg := &ctrl_pb.Link{Id: link.Id.Token}
				body, err := proto.Marshal(linkMsg)
				if err == nil {
					msg := channel2.NewMessage(int32(ctrl_pb.ContentType_LinkType), body)
					if err := self.ctrl.Channel().Send(msg); err == nil {
						log.Infof("link [l/%s] up", link.Id.Token)
					} else {
						log.Errorf("unexpected error sending link (%s)", err)
					}

				} else {
					log.Errorf("unexpected error (%s)", err)
				}
			} else {
				log.Errorf("error binding link (%s)", err)
			}
		} else {
			log.Errorf("link dialing failed [%s]", address.String())

			fault := &ctrl_pb.Fault{Subject: ctrl_pb.FaultSubject_LinkFault, Id: linkId.Token}
			body, err := proto.Marshal(fault)
			if err == nil {
				msg := channel2.NewMessage(int32(ctrl_pb.ContentType_FaultType), body)
				if err := self.ctrl.Channel().Send(msg); err != nil {
					log.Errorf("error sending fault (%s)", err)
				}
			} else {
				log.Errorf("unexpected error (%s)", err)
			}
		}
	} else {
		log.Errorf("link address parsing failed (%s)", err)
	}

	return nil
}

func (_ *impl) GetAdvertisement() string {
	return ""
}

type impl struct {
	id               *identity.TokenId
	config           *config
	listener         channel2.UnderlayListener
	ctrl             xgress.CtrlChannel
	forwarder        *forwarder.Forwarder
	forwarderOptions *forwarder.Options
	metricsRegistry  metrics.Registry
	linkOptions      *channel2.Options
}
