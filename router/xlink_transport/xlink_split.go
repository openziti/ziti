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
	"github.com/openziti/metrics"
	"github.com/pkg/errors"
	"sync/atomic"
)

type splitImpl struct {
	id              string
	payloadCh       channel.Channel
	ackCh           channel.Channel
	routerId        string
	routerVersion   string
	linkProtocol    string
	dialAddress     string
	closeNotified   atomic.Bool
	droppedMsgMeter metrics.Meter
}

func (self *splitImpl) Id() string {
	return self.id
}

func (self *splitImpl) SendPayload(msg *xgress.Payload) error {
	sent, err := self.payloadCh.TrySend(msg.Marshall())
	if err == nil && !sent {
		self.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *splitImpl) SendAcknowledgement(msg *xgress.Acknowledgement) error {
	sent, err := self.ackCh.TrySend(msg.Marshall())
	if err == nil && !sent {
		self.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *splitImpl) SendControl(msg *xgress.Control) error {
	sent, err := self.payloadCh.TrySend(msg.Marshall())
	if err == nil && !sent {
		self.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *splitImpl) CloseNotified() error {
	self.closeNotified.Store(true)
	return self.Close()
}

func (self *splitImpl) Close() error {
	var err, err2 error
	if self.payloadCh != nil {
		err = self.payloadCh.Close()
	}

	if self.ackCh != nil {
		err2 = self.ackCh.Close()
	}
	if err == nil {
		return err2
	}
	if err2 == nil {
		return err
	}
	return errors.Errorf("multiple failures while closing transport link (%v) (%v)", err, err2)
}

func (self *splitImpl) DestinationId() string {
	return self.routerId
}

func (self *splitImpl) DestVersion() string {
	return self.routerVersion
}

func (self *splitImpl) LinkProtocol() string {
	return self.linkProtocol
}

func (self *splitImpl) DialAddress() string {
	return self.dialAddress
}

func (self *splitImpl) HandleCloseNotification(f func()) {
	if self.closeNotified.CompareAndSwap(false, true) {
		f()
	}
}

func (self *splitImpl) IsClosed() bool {
	return self.payloadCh.IsClosed() || self.ackCh.IsClosed()
}

func (self *splitImpl) InspectCircuit(detail *inspect.CircuitInspectDetail) {
	detail.LinkDetails[self.id] = self.InspectLink()
}

func (self *splitImpl) InspectLink() *inspect.LinkInspectDetail {
	return &inspect.LinkInspectDetail{
		Id:          self.Id(),
		Split:       true,
		Protocol:    self.LinkProtocol(),
		DialAddress: self.DialAddress(),
		Dest:        self.DestinationId(),
		DestVersion: self.DestVersion(),
	}
}

func (self *splitImpl) GetAddresses() []*ctrl_pb.LinkConn {
	ackLocalAddr := self.ackCh.Underlay().GetLocalAddr()
	ackRemoteAddr := self.ackCh.Underlay().GetRemoteAddr()

	plLocalAddr := self.payloadCh.Underlay().GetLocalAddr()
	plRemoteAddr := self.payloadCh.Underlay().GetRemoteAddr()

	return []*ctrl_pb.LinkConn{
		{
			Id:         "ack",
			LocalAddr:  ackLocalAddr.Network() + ":" + ackLocalAddr.String(),
			RemoteAddr: ackRemoteAddr.Network() + ":" + ackRemoteAddr.String(),
		},
		{
			Id:         "payload",
			LocalAddr:  plLocalAddr.Network() + ":" + plLocalAddr.String(),
			RemoteAddr: plRemoteAddr.Network() + ":" + plRemoteAddr.String(),
		},
	}
}
