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
	"github.com/openziti/channel"
	"github.com/openziti/fabric/inspect"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/identity"
)

type impl struct {
	id            *identity.TokenId
	ch            channel.Channel
	routerId      string
	routerVersion string
	linkProtocol  string
	dialAddress   string
	closeNotified concurrenz.AtomicBoolean
}

func (self *impl) Id() *identity.TokenId {
	return self.id
}

func (self *impl) SendPayload(payload *xgress.Payload) error {
	return self.ch.Send(payload.Marshall())
}

func (self *impl) SendAcknowledgement(acknowledgement *xgress.Acknowledgement) error {
	return self.ch.Send(acknowledgement.Marshall())
}

func (self *impl) SendControl(control *xgress.Control) error {
	return self.ch.Send(control.Marshall())
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
	detail.LinkDetails[self.id.Token] = self.InspectLink()
}

func (self *impl) InspectLink() *inspect.LinkInspectDetail {
	return &inspect.LinkInspectDetail{
		Id:          self.Id().Token,
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
