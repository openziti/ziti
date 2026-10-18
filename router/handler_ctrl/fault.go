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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/xlink"
	"google.golang.org/protobuf/proto"
)

type faultHandler struct {
	xlinkRegistry xlink.Registry
	env           env.RouterEnv
	poolFallbacks metrics.Meter
}

func newFaultHandler(routerEnv env.RouterEnv) *faultHandler {
	return &faultHandler{
		xlinkRegistry: routerEnv.GetXlinkRegistry(),
		env:           routerEnv,
		poolFallbacks: routerEnv.GetMetricsRegistry().Meter("link.fault.rx_pool_fallback"),
	}
}

func (self *faultHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_FaultType)
}

func (self *faultHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label())

	fault := &ctrl_pb.Fault{}
	if err := proto.Unmarshal(msg.Body, fault); err != nil {
		log.WithError(err).Error("failed to unmarshal fault message")
		return
	}

	handle := func() { self.handleFault(msg, ch, fault) }

	// Bounded while the receive pool has room, unbounded when it does not. Spawning is the fallback rather
	// than the default because a dropped fault leaves a dead link marked up with no repair loop behind it,
	// and because waiting for pool capacity would put the burst on this goroutine, which also carries every
	// other default priority message on this underlay. The pool is resolved here rather than at construction
	// since handlers are built before the pools exist.
	if pool := self.env.GetRxPool(); pool != nil && pool.QueueOrError(handle) == nil {
		return
	}

	self.poolFallbacks.Mark(1)
	go handle()
}

func (self *faultHandler) handleFault(_ *channel.Message, ch channel.Channel, fault *ctrl_pb.Fault) {
	log := pfxlog.ContextLogger(ch.Label()).Entry

	switch fault.Subject {
	case ctrl_pb.FaultSubject_LinkFault:
		linkId := fault.Id
		log = log.WithField("linkId", linkId)
		if link, _ := self.xlinkRegistry.GetLinkById(linkId); link != nil {
			if fault.Iteration > 0 && fault.Iteration < link.Iteration() {
				log.WithField("fault.iteration", fault.Iteration).
					WithField("link.iteration", link.Iteration()).
					Info("link fault reported, but fault iteration < link iteration, ignoring")
				return
			}
			log.Info("link fault reported, closing")
			if err := link.CloseNotified(); err != nil {
				log.WithError(err).Error("failure closing link")
			}
		} else {
			log.Info("link fault reported, link already closed or unknown")
		}

	default:
		log.WithField("subject", fault.Subject.String()).Error("unhandled fault subject")
	}
}
