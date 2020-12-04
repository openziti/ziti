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

package handler_link

import (
	"github.com/ef-ds/deque"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/router/forwarder"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/fabric/router/xlink"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/metrics"
	"sync/atomic"
	"time"
)

type queuingAckHandler struct {
	link          xlink.Xlink
	ctrl          xgress.CtrlChannel
	forwarder     *forwarder.Forwarder
	timer         metrics.Timer
	acks          *deque.Deque
	ackIngest     chan *xgress.Acknowledgement
	ackForward    chan *xgress.Acknowledgement
	acksQueueSize int64
}

func newQueuingAckHandler(link xlink.Xlink, ctrl xgress.CtrlChannel, forwarder *forwarder.Forwarder) *queuingAckHandler {
	result := &queuingAckHandler{
		link:       link,
		ctrl:       ctrl,
		forwarder:  forwarder,
		timer:      forwarder.MetricsRegistry().Timer("xgress.ack.handle_time"),
		acks:       deque.New(),
		ackIngest:  make(chan *xgress.Acknowledgement, 16),
		ackForward: make(chan *xgress.Acknowledgement, 1),
	}

	go result.ackIngester()
	go result.ackForwarder()

	forwarder.MetricsRegistry().FuncGauge("xgress.acks.queue_size", func() int64 {
		return atomic.LoadInt64(&result.acksQueueSize)
	})

	return result
}

func (self *queuingAckHandler) ContentType() int32 {
	return xgress.ContentTypeAcknowledgementType
}

func (self *queuingAckHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	start := time.Now()
	log := pfxlog.ContextLogger(ch.Label())

	if ack, err := xgress.UnmarshallAcknowledgement(msg); err == nil {
		self.ackIngest <- ack
	} else {
		log.Errorf("unexpected error (%v)", err)
	}
	self.timer.UpdateSince(start)
}

func (self *queuingAckHandler) ackIngester() {
	var next *xgress.Acknowledgement
	for {
		if next == nil {
			if val, _ := self.acks.PopFront(); val != nil {
				next = val.(*xgress.Acknowledgement)
			}
		}

		if next == nil {
			select {
			case ack := <-self.ackIngest:
				self.acks.PushBack(ack)
			}
		} else {
			select {
			case ack := <-self.ackIngest:
				self.acks.PushBack(ack)
			case self.ackForward <- next:
				next = nil
			}
		}
		atomic.StoreInt64(&self.acksQueueSize, int64(self.acks.Len()))
	}
}

func (self *queuingAckHandler) ackForwarder() {
	logger := pfxlog.Logger()
	for ack := range self.ackForward {
		now := time.Now()
		if err := self.forwarder.ForwardAcknowledgement(xgress.Address(self.link.Id().Token), ack); err != nil {
			logger.WithError(err).Debugf("unable to forward acknowledgement (%v)", err)
		} else {
			self.timer.UpdateSince(now)
		}
	}
}
