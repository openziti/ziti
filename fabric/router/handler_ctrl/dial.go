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

package handler_ctrl

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-fabric/fabric/metrics"
	"github.com/netfoundry/ziti-fabric/fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-fabric/fabric/router/forwarder"
	"github.com/netfoundry/ziti-fabric/fabric/router/handler_link"
	"github.com/netfoundry/ziti-fabric/fabric/xgress"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
)

type dialHandler struct {
	id               *identity.TokenId
	ctrl             xgress.CtrlChannel
	linkOptions      *channel2.Options
	forwarderOptions *forwarder.Options
	forwarder        *forwarder.Forwarder
	metricsRegistry  metrics.Registry
}

func newDialHandler(id *identity.TokenId, ctrl xgress.CtrlChannel, linkOptions *channel2.Options, forwarderOptions *forwarder.Options, forwarder *forwarder.Forwarder, metricsRegistry metrics.Registry) *dialHandler {
	return &dialHandler{
		id:               id,
		ctrl:             ctrl,
		linkOptions:      linkOptions,
		forwarderOptions: forwarderOptions,
		forwarder:        forwarder,
		metricsRegistry:  metricsRegistry,
	}
}

func (h *dialHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_DialType)
}

func (h *dialHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	log := pfxlog.ContextLogger(ch.Label())
	log.Info("received link connect request")

	dial := &ctrl_pb.Dial{}
	if err := proto.Unmarshal(msg.Body, dial); err == nil {
		address, err := transport.ParseAddress(dial.Address)
		if err == nil {
			linkId := h.id.ShallowCloneWithNewToken(dial.Id)
			name := "l/" + linkId.Token

			dialer := channel2.NewClassicDialer(linkId, address, nil)
			ch, err := channel2.NewChannel(name, dialer, h.linkOptions)
			if err == nil {
				link := forwarder.NewLink(linkId, ch)
				if err := link.Channel.Bind(handler_link.NewBindHandler(h.id, link, h.ctrl, h.forwarder)); err == nil {
					h.forwarder.RegisterLink(link)

					go metrics.ProbeLatency(
						link.Channel,
						h.metricsRegistry.Histogram("link."+linkId.Token+".latency"),
						h.forwarderOptions.LatencyProbeInterval,
					)

					linkMsg := &ctrl_pb.Link{Id: link.Id.Token}
					body, err := proto.Marshal(linkMsg)
					if err == nil {
						msg := channel2.NewMessage(int32(ctrl_pb.ContentType_LinkType), body)
						if err := h.ctrl.Channel().Send(msg); err == nil {
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
					if err := h.ctrl.Channel().Send(msg); err != nil {
						log.Errorf("error sending fault (%s)", err)
					}
				} else {
					log.Errorf("unexpected error (%s)", err)
				}
			}
		} else {
			log.Errorf("link address parsing failed (%s)", err)
		}

	} else {
		log.Errorf("unexpected error (%s)", err)
	}
}
