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
		QueueSize:   1, // The actual work queue is now external to the pool
		MinWorkers:  0,
		MaxWorkers:  64, // workers handle ordering internally
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

// processMessages processes all available messages in the queue until empty.
// This method is called by worker goroutines from the pool to handle batches of commands.
func (self *commandHandler) processMessages() {
	for self.processMessage() {
		// process until we run out of messages
	}
}

// processMessage processes a single message from the queue using a two-phase approach.
//
// The two-phase approach is critical for separating rate limiting concerns:
//
// Phase 1 (under lock):
//   - Dequeue the next message from the queue
//   - Acquire a slot in the adaptive rate limiter (raft.rateLimiter)
//   - Submit the command to Raft.Apply(), which enqueues it in Raft's internal queue
//   - Return a continuation function (phaseTwo) for later execution
//
// Phase 2 (lock released):
//   - Execute the continuation function to wait for Raft processing
//   - Block until the command is applied to the distributed log
//   - Send response back to the caller
//
// Why use two phases?
//
// 1. Rate Limiting Separation:
//    - The adaptive rate limiter (raft.rateLimiter) controls how many operations are
//      IN-FLIGHT (waiting for Raft consensus), preventing overwhelming the Raft subsystem
//    - The queue (self.queue) controls how many operations are SUBMITTED (pending rate limiting)
//    - This separation allows for independent tuning of submission vs. in-flight limits
//
// 2. Prevents Queue Starvation:
//    - Phase 1 holds the lock only long enough to dequeue and submit to Raft
//    - Phase 2 releases the lock while waiting for Raft consensus (potentially seconds)
//    - This allows other workers to dequeue and submit their commands to Raft concurrently
//    - Without this, one slow Raft operation would block all other operations from even
//      being submitted, leading to queue buildup and timeouts
//
// 3. Maximizes Raft Throughput:
//    - Multiple operations can be submitted to Raft's internal queue quickly in succession
//    - Raft can then batch and process these operations more efficiently
//    - Workers block waiting for results outside the critical section
//
// 4. Fair Processing:
//    - Commands are dequeued in order (Phase 1) but can complete in any order (Phase 2)
//    - This prevents head-of-line blocking where a single slow command delays all others
//
// Example scenario without two-phase:
//   - Worker 1 dequeues msg A, holds lock, calls Raft.Apply(), waits 5s for consensus
//   - Worker 2 wants to process msg B but is blocked waiting for Worker 1's lock
//   - Queue fills up with msgs C, D, E... all waiting for Worker 1 to finish
//   - Result: Poor throughput, timeouts, and backpressure
//
// Example scenario with two-phase:
//   - Worker 1 dequeues msg A, submits to Raft, releases lock, waits for result
//   - Worker 2 dequeues msg B, submits to Raft, releases lock, waits for result
//   - Worker 3 dequeues msg C, submits to Raft, releases lock, waits for result
//   - All three are now in Raft's queue, being processed concurrently
//   - Result: High throughput, efficient Raft batching, better resource utilization
func (self *commandHandler) processMessage() bool {
	var pair msgAndChannel
	var phaseTwo func() (interface{}, uint64, error)
	var err error

	// Phase 1: Dequeue and submit to Raft (under lock)
	self.lock.Lock()
	select {
	case pair = <-self.queue:
		// ApplyTwoPhase acquires a rate limiter slot and submits to Raft.Apply()
		// It returns immediately with a continuation function, not waiting for Raft consensus
		phaseTwo, err = self.controller.ApplyTwoPhase(pair.msg.Body)
	default:
		self.lock.Unlock()
		return false
	}
	self.lock.Unlock()

	if err != nil {
		// Rate limiter rejected the operation (too many in-flight operations)
		sendErrorResponseCalculateType(pair.msg, pair.ch, apierror.NewTooManyUpdatesError())
		return true
	}

	// Phase 2: Wait for Raft consensus and send response (lock released)
	// This can take seconds, but other workers can continue processing during this time
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
