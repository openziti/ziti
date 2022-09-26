/*
	(c) Copyright NetFoundry Inc.

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
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/inspect"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/metrics"
)

type impl struct {
	id              string
	ch              channel.Channel
	routerId        string
	routerVersion   string
	linkProtocol    string
	dialAddress     string
	closeNotified   concurrenz.AtomicBoolean
	droppedMsgMeter metrics.Meter
}

func (self *impl) Id() string {
	return self.id
}

func (self *impl) SendPayload(msg *xgress.Payload) error {
	sent, err := self.ch.TrySend(msg.Marshall())
	if err == nil && !sent {
		self.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *impl) SendAcknowledgement(msg *xgress.Acknowledgement) error {
	sent, err := self.ch.TrySend(msg.Marshall())
	if err == nil && !sent {
		self.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *impl) SendControl(msg *xgress.Control) error {
	sent, err := self.ch.TrySend(msg.Marshall())
	if err == nil && !sent {
		self.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *impl) Close() error {
	return self.ch.Close()
}

func (self *impl) CloseNotified() error {
	self.closeNotified.Set(true)
	return self.Close()
}

func (self *impl) DestinationId() string {
	return self.routerId
}

func (self *impl) DestVersion() string {
	return self.routerVersion
}

func (self *impl) LinkProtocol() string {
	return self.linkProtocol
}

func (self *impl) DialAddress() string {
	return self.dialAddress
}

func (self *impl) HandleCloseNotification(f func()) {
	if self.closeNotified.CompareAndSwap(false, true) {
		f()
	}
}

func (self *impl) IsClosed() bool {
	return self.ch.IsClosed()
}

func (self *impl) InspectCircuit(detail *inspect.CircuitInspectDetail) {
	detail.LinkDetails[self.id] = self.InspectLink()
}

func (self *impl) InspectLink() *inspect.LinkInspectDetail {
	return &inspect.LinkInspectDetail{
		Id:          self.Id(),
		Split:       false,
		Protocol:    self.LinkProtocol(),
		DialAddress: self.DialAddress(),
		Dest:        self.DestinationId(),
		DestVersion: self.DestVersion(),
	}
}

func (self *impl) GetAddresses() []*ctrl_pb.LinkConn {
	localAddr := self.ch.Underlay().GetLocalAddr()
	remoteAddr := self.ch.Underlay().GetRemoteAddr()
	return []*ctrl_pb.LinkConn{
		{
			Id:         "single",
			LocalAddr:  localAddr.Network() + ":" + localAddr.String(),
			RemoteAddr: remoteAddr.Network() + ":" + remoteAddr.String(),
		},
	}
}
