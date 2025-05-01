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
	"github.com/openziti/channel/v4"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/common/inspect"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/pkg/errors"
	"sync"
	"sync/atomic"
	"time"
)

type splitImpl struct {
	id            string
	key           string
	payloadCh     channel.Channel
	ackCh         channel.Channel
	routerId      string
	routerVersion string
	linkProtocol  string
	dialAddress   string
	closed        atomic.Bool
	faultsSent    atomic.Bool
	dialed        bool
	iteration     uint32
	dupsRejected  uint32
	lock          sync.Mutex

	droppedMsgMeter    metrics.Meter
	droppedXgMsgMeter  metrics.Meter
	droppedRtxMsgMeter metrics.Meter
	droppedFwdMsgMeter metrics.Meter
}

func (self *splitImpl) Id() string {
	return self.id
}

func (self *splitImpl) Key() string {
	return self.key
}

func (self *splitImpl) Iteration() uint32 {
	return self.iteration
}

func (self *splitImpl) Init(metricsRegistry metrics.Registry) error {
	if self.droppedMsgMeter == nil {
		self.droppedMsgMeter = metricsRegistry.Meter("link.dropped_msgs:" + self.id)
		self.droppedXgMsgMeter = metricsRegistry.Meter("link.dropped_xg_msgs:" + self.id)
		self.droppedRtxMsgMeter = metricsRegistry.Meter("link.dropped_rtx_msgs:" + self.id)
		self.droppedFwdMsgMeter = metricsRegistry.Meter("link.dropped_fwd_msgs:" + self.id)
	}
	return nil
}

func (self *splitImpl) syncInit(f func() error) error {
	self.lock.Lock()
	defer self.lock.Unlock()
	return f()
}

func (self *splitImpl) SendPayload(msg *xgress.Payload, timeout time.Duration, payloadType xgress.PayloadType) error {
	if timeout == 0 {
		sent, err := self.payloadCh.TrySend(msg.Marshall())
		if err == nil && !sent {
			self.droppedMsgMeter.Mark(1)
			if payloadType == xgress.PayloadTypeXg {
				self.droppedXgMsgMeter.Mark(1)
			} else if payloadType == xgress.PayloadTypeRtx {
				self.droppedRtxMsgMeter.Mark(1)
			} else if payloadType == xgress.PayloadTypeFwd {
				self.droppedFwdMsgMeter.Mark(1)
			}
		}
		return err
	}

	return msg.Marshall().WithTimeout(timeout).Send(self.payloadCh)
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
	self.faultsSent.Store(true)
	return self.Close()
}

func (self *splitImpl) AreFaultsSent() bool {
	return self.faultsSent.Load()
}

func (self *splitImpl) Close() error {
	self.lock.Lock()
	defer self.lock.Unlock()

	if self.droppedMsgMeter != nil {
		self.droppedMsgMeter.Dispose()
	}
	var err, err2 error
	if ch := self.payloadCh; ch != nil {
		err = ch.Close()
	}

	if ch := self.ackCh; ch != nil {
		err2 = ch.Close()
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

func (self *splitImpl) CloseOnce(f func()) {
	if self.closed.CompareAndSwap(false, true) {
		f()
	}
}

func (self *splitImpl) IsClosed() bool {
	return self.payloadCh.IsClosed() || self.ackCh.IsClosed()
}

func (self *splitImpl) IsDialed() bool {
	return self.dialed
}

func (self *splitImpl) InspectCircuit(detail *xgress.CircuitInspectDetail) {
	detail.AddRelatedEntity("link", self.id, self.InspectLink())
}

func (self *splitImpl) InspectLink() *inspect.LinkInspectDetail {
	return &inspect.LinkInspectDetail{
		Id:          self.Id(),
		Iteration:   self.Iteration(),
		Key:         self.key,
		Split:       true,
		Protocol:    self.LinkProtocol(),
		DialAddress: self.DialAddress(),
		Dest:        self.DestinationId(),
		DestVersion: self.DestVersion(),
		Dialed:      self.dialed,
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

func (self *splitImpl) DuplicatesRejected() uint32 {
	return atomic.AddUint32(&self.dupsRejected, 1)
}
