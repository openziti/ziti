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
	"github.com/openziti/ziti/router/xlink"
	"sync/atomic"
	"time"
)

type impl struct {
	id            string
	key           string
	ch            LinkChannel
	routerId      string
	routerVersion string
	linkProtocol  string
	dialAddress   string
	closed        atomic.Bool
	faultsSent    atomic.Bool
	dialed        bool
	iteration     uint32
	dupsRejected  uint32

	droppedMsgMeter    metrics.Meter
	droppedXgMsgMeter  metrics.Meter
	droppedRtxMsgMeter metrics.Meter
	droppedFwdMsgMeter metrics.Meter
}

func (self *impl) Id() string {
	return self.id
}

func (self *impl) Key() string {
	return self.key
}

func (self *impl) Iteration() uint32 {
	return self.iteration
}

func (self *impl) GetDestinationType() string {
	return "link"
}

func (self *impl) Init(metricsRegistry metrics.Registry) error {
	if self.droppedMsgMeter == nil {
		self.droppedMsgMeter = metricsRegistry.Meter("link.dropped_msgs:" + self.id)
		self.droppedXgMsgMeter = metricsRegistry.Meter("link.dropped_xg_msgs:" + self.id)
		self.droppedRtxMsgMeter = metricsRegistry.Meter("link.dropped_rtx_msgs:" + self.id)
		self.droppedFwdMsgMeter = metricsRegistry.Meter("link.dropped_fwd_msgs:" + self.id)
	}
	return nil
}

func (self *impl) SendPayload(msg *xgress.Payload, timeout time.Duration, payloadType xgress.PayloadType) error {
	if timeout == 0 {
		sent, err := self.ch.GetDefaultSender().TrySend(msg.Marshall())
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

	return msg.Marshall().WithTimeout(timeout).Send(self.ch.GetDefaultSender())
}

func (self *impl) SendAcknowledgement(msg *xgress.Acknowledgement) error {
	sent, err := self.ch.GetAckSender().TrySend(msg.Marshall())
	if err == nil && !sent {
		self.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *impl) SendControl(msg *xgress.Control) error {
	sent, err := self.ch.GetDefaultSender().TrySend(msg.Marshall())
	if err == nil && !sent {
		self.droppedMsgMeter.Mark(1)
	}
	return err
}

func (self *impl) Close() error {
	self.droppedMsgMeter.Dispose()
	return self.ch.GetChannel().Close()
}

func (self *impl) CloseNotified() error {
	self.faultsSent.Store(true)
	return self.Close()
}

func (self *impl) AreFaultsSent() bool {
	return self.faultsSent.Load()
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

func (self *impl) IsDialed() bool {
	return self.dialed
}

func (self *impl) CloseOnce(f func()) {
	if self.closed.CompareAndSwap(false, true) {
		f()
	}
}

func (self *impl) IsClosed() bool {
	return self.ch.GetChannel().IsClosed()
}

func (self *impl) InspectCircuit(detail *xgress.CircuitInspectDetail) {
	detail.AddRelatedEntity("link", self.id, self.InspectLink())
}

func (self *impl) InspectLink() *inspect.LinkInspectDetail {
	result := GetLinkInspectDetail(self)
	result.Split = false
	result.Underlays = self.ch.GetChannel().GetUnderlayCountsByType()
	return result
}

func (self *impl) GetLinkConnState() *ctrl_pb.LinkConnState {
	var connections []*ctrl_pb.LinkConn
	for _, u := range self.ch.GetChannel().GetUnderlays() {
		localAddr := u.GetLocalAddr()
		remoteAddr := u.GetRemoteAddr()
		t := channel.GetUnderlayType(u)
		if t == "" {
			t = "single"
		}
		connections = append(connections, &ctrl_pb.LinkConn{
			Type:       t,
			LocalAddr:  localAddr.Network() + ":" + localAddr.String(),
			RemoteAddr: remoteAddr.Network() + ":" + remoteAddr.String(),
		})
	}
	return &ctrl_pb.LinkConnState{
		StateIteration: self.ch.GetConnStateIteration(),
		Conns:          connections,
	}
}

func (self *impl) DuplicatesRejected() uint32 {
	return atomic.AddUint32(&self.dupsRejected, 1)
}

func (self *impl) MarkLinkStateSynced(ctrlId string) {
	if stateConn, ok := self.ch.(StateTrackingLinkChannel); ok {
		stateConn.MarkLinkStateSynced(ctrlId)
	}
}

func (self *impl) MarkLinkStateSyncedForState(ctrlId string, stateId string) {
	if stateConn, ok := self.ch.(StateTrackingLinkChannel); ok {
		stateConn.MarkLinkStateSyncedForState(ctrlId, stateId)
	}
}

func (self *impl) GetCtrlRequiringSync() (string, []string) {
	if stateConn, ok := self.ch.(StateTrackingLinkChannel); ok {
		return stateConn.GetCtrlRequiringSync()
	}
	return "", nil
}

func GetLinkInspectDetail(link xlink.Xlink) *inspect.LinkInspectDetail {
	result := &inspect.LinkInspectDetail{
		Id:          link.Id(),
		Iteration:   link.Iteration(),
		Key:         link.Key(),
		Protocol:    link.LinkProtocol(),
		DialAddress: link.DialAddress(),
		Dest:        link.DestinationId(),
		DestVersion: link.DestVersion(),
		Dialed:      link.IsDialed(),
	}

	linkConnState := link.GetLinkConnState()
	result.ConnStateIteration = linkConnState.StateIteration
	for _, c := range linkConnState.Conns {
		result.Connections = append(result.Connections, &inspect.LinkConnection{
			Type:   c.Type,
			Source: c.LocalAddr,
			Dest:   c.RemoteAddr,
		})
	}

	return result
}
