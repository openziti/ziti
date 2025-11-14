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

package handler_peer_ctrl

import (
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/ziti/common/metrics"
	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/controller/apierror"
	"github.com/openziti/ziti/controller/raft"
	"github.com/sirupsen/logrus"
)

func newCommandHandler(controller *raft.Controller) channel.TypedReceiveHandler {
	poolConfig := goroutines.PoolConfig{
		QueueSize:   1,
		MinWorkers:  0,
		MaxWorkers:  64, // we should only have one thing apply entries, so they don't get applied out of order
		IdleTime:    time.Second,
		CloseNotify: controller.GetCloseNotify(),
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().WithField(logrus.ErrorKey, err).Error("panic during command processing")
		},
		WorkerFunction: commandHandlerWorker,
	}
	metrics.ConfigureGoroutinesPoolMetrics(&poolConfig, controller.GetMetricsRegistry(), "command_handler")
	pool, err := goroutines.NewPool(poolConfig)
	if err != nil {
		panic(err)
	}
	return &commandHandler{
		controller: controller,
		pool:       pool,
		queue:      make(chan msgAndChannel, controller.Config.CommandHandlerOptions.MaxQueueSize),
	}
}

func commandHandlerWorker(_ uint32, f func()) {
	f()
}

type commandHandler struct {
	controller *raft.Controller
	pool       goroutines.Pool
	queue      chan msgAndChannel
	lock       sync.Mutex
}

func (self *commandHandler) ContentType() int32 {
	return int32(cmd_pb.ContentType_NewLogEntryType)
}

func (self *commandHandler) processMessages() {
	for self.processMessage() {
		// process until we run out of messages
	}
}

func (self *commandHandler) processMessage() bool {
	var pair msgAndChannel
	var phaseTwo func() (interface{}, uint64, error)
	var err error

	self.lock.Lock()
	select {
	case pair = <-self.queue:
		phaseTwo, err = self.controller.ApplyTwoPhase(pair.msg.Body)
	default:
		self.lock.Unlock()
		return false
	}
	self.lock.Unlock()

	if err != nil {
		sendErrorResponseCalculateType(pair.msg, pair.ch, apierror.NewTooManyUpdatesError())
		return true
	}

	result, index, err := phaseTwo()
	if index, err = self.controller.HandleApplyOutput(pair.msg.Body, result, index, err); err != nil {
		sendErrorResponseCalculateType(pair.msg, pair.ch, err)
	} else {
		sendSuccessResponse(pair.msg, pair.ch, index)
	}

	return true
}

func (self *commandHandler) HandleReceive(m *channel.Message, ch channel.Channel) {
	select {
	case self.queue <- msgAndChannel{msg: m, ch: ch}:
	default:
		go sendErrorResponseCalculateType(m, ch, apierror.NewTooManyUpdatesError())
		return
	}

	// we don't care if the queue is full, another worker will pick the work up
	_ = self.pool.QueueOrError(self.processMessages)
}

type msgAndChannel struct {
	msg *channel.Message
	ch  channel.Channel
}
