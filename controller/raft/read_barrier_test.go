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

package raft

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/pb/cmd_pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// fakeBarrierSource stands in for raft, so taking a barrier can be exercised without a cluster.
type fakeBarrierSource struct {
	leaderAndCaughtUp bool
	barrierErr        error
	appliedIndex      uint64
	barrierCalls      atomic.Int32
	// barrierBlock, when set, holds the barrier until it is closed, standing in for waiting on quorum.
	barrierBlock chan struct{}
}

func (self *fakeBarrierSource) IsLeaderAndCaughtUp() bool { return self.leaderAndCaughtUp }

func (self *fakeBarrierSource) GetAppliedIndex() uint64 { return self.appliedIndex }

func (self *fakeBarrierSource) barrier(time.Duration) error {
	self.barrierCalls.Add(1)
	if self.barrierBlock != nil {
		<-self.barrierBlock
	}
	return self.barrierErr
}

func (self *fakeBarrierSource) take() (uint64, error) {
	return takeReadBarrier(self)
}

// recordingChannel captures what a handler sends. Only Send and Id are reached; anything else would be a
// change in how the handler answers, which should fail loudly rather than silently pass.
type recordingChannel struct {
	channel.Channel
	sent chan *channel.Message
}

func newRecordingChannel() *recordingChannel {
	return &recordingChannel{sent: make(chan *channel.Message, 4)}
}

func (self *recordingChannel) Send(s channel.Sendable) error {
	self.sent <- s.Msg()
	return nil
}

func (self *recordingChannel) Id() string { return "peer" }

func (self *recordingChannel) await(t *testing.T) *channel.Message {
	t.Helper()
	select {
	case msg := <-self.sent:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("handler sent nothing")
		return nil
	}
}

func readBarrierRequestMsg(requestId uint64) *channel.Message {
	return protobufs.MarshalTyped(&cmd_pb.ReadBarrierRequest{RequestId: requestId}).ToSendable().Msg()
}

func readBarrierResponseMsg(requestId, index uint64) *channel.Message {
	return protobufs.MarshalTyped(&cmd_pb.ReadBarrierResponse{RequestId: requestId, Index: index}).ToSendable().Msg()
}

// leaderStub records what a follower sends to the leader.
type leaderStub struct {
	lock     sync.Mutex
	requests []uint64
	err      error
}

func (self *leaderStub) send(request *cmd_pb.ReadBarrierRequest) error {
	self.lock.Lock()
	defer self.lock.Unlock()
	if self.err != nil {
		return self.err
	}
	self.requests = append(self.requests, request.RequestId)
	return nil
}

func (self *leaderStub) sent() []uint64 {
	self.lock.Lock()
	defer self.lock.Unlock()
	return append([]uint64(nil), self.requests...)
}

func newFollowerBarriers(leader *leaderStub) *readBarriers {
	return newReadBarriers(func() bool { return false }, func() (uint64, error) {
		panic("a follower does not take its own barrier")
	}, leader.send)
}

func (self *readBarriers) pendingRequest() pendingReadBarrier {
	self.lock.Lock()
	defer self.lock.Unlock()
	return self.pending
}

// Test_ReadBarrierRequest_AnswersWithTheIndexAfterTheBarrier: the index is only a barrier if it is read
// after one, and the requester matches the answer to its request by the echoed id.
func Test_ReadBarrierRequest_AnswersWithTheIndexAfterTheBarrier(t *testing.T) {
	req := require.New(t)
	src := &fakeBarrierSource{leaderAndCaughtUp: true, appliedIndex: 4242}
	handler := &readBarrierRequestHandler{take: src.take}
	ch := newRecordingChannel()

	handler.HandleReceive(readBarrierRequestMsg(17), ch)

	reply := ch.await(t)
	req.Equal(int32(cmd_pb.ContentType_ReadBarrierResponseType), reply.ContentType)

	response := &cmd_pb.ReadBarrierResponse{}
	req.NoError(proto.Unmarshal(reply.Body, response))
	req.Equal(uint64(17), response.RequestId)
	req.Equal(uint64(4242), response.Index)
	req.Equal(int32(1), src.barrierCalls.Load(), "the barrier is what makes the index mean anything")
}

// Test_ReadBarrierRequest_SendsNothingWhenNotCaughtUpWithItsTerm: leadership begins when an election is won,
// before the applies that make the local view current. An index read in that window can omit entries
// committed in the previous term.
func Test_ReadBarrierRequest_SendsNothingWhenNotCaughtUpWithItsTerm(t *testing.T) {
	req := require.New(t)
	src := &fakeBarrierSource{leaderAndCaughtUp: false, appliedIndex: 4242}
	handler := &readBarrierRequestHandler{take: src.take}
	ch := newRecordingChannel()

	handler.answer(17, ch)

	req.Empty(ch.sent)
	req.Zero(src.barrierCalls.Load(), "and must not reach the barrier at all")
}

// Test_ReadBarrierRequest_SendsNothingWhenTheBarrierFails: the barrier commits through quorum, so it fails
// for a leader partitioned away without knowing it, whose index would be stale.
func Test_ReadBarrierRequest_SendsNothingWhenTheBarrierFails(t *testing.T) {
	req := require.New(t)
	src := &fakeBarrierSource{leaderAndCaughtUp: true, appliedIndex: 4242, barrierErr: errors.New("leadership lost")}
	handler := &readBarrierRequestHandler{take: src.take}
	ch := newRecordingChannel()

	handler.answer(17, ch)

	req.Empty(ch.sent, "a leader that cannot prove it still leads must not answer")
	req.Equal(int32(1), src.barrierCalls.Load())
}

// Test_ReadBarrierRequest_DoesNotAnswerOnTheReceiveGoroutine: the barrier waits on quorum, and the receive
// goroutine carries every other message on the channel, heartbeats included.
func Test_ReadBarrierRequest_DoesNotAnswerOnTheReceiveGoroutine(t *testing.T) {
	req := require.New(t)
	release := make(chan struct{})
	src := &fakeBarrierSource{leaderAndCaughtUp: true, appliedIndex: 7, barrierBlock: release}
	handler := &readBarrierRequestHandler{take: src.take}
	ch := newRecordingChannel()

	returned := make(chan struct{})
	go func() {
		handler.HandleReceive(readBarrierRequestMsg(3), ch)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		req.FailNow("HandleReceive blocked on the barrier")
	}

	req.Empty(ch.sent, "nothing is sent until the barrier completes")

	close(release)
	reply := ch.await(t)
	req.Equal(int32(cmd_pb.ContentType_ReadBarrierResponseType), reply.ContentType)
}

// Test_ReadBarriers_FollowerAsksTheLeaderAndKeepsTheRequestTime: the answer can only speak for what was
// committed before the request, so it is recorded against the request's time, not the answer's.
func Test_ReadBarriers_FollowerAsksTheLeaderAndKeepsTheRequestTime(t *testing.T) {
	req := require.New(t)
	leader := &leaderStub{}
	barriers := newFollowerBarriers(leader)
	handler := &readBarrierResponseHandler{barriers: barriers}

	req.True(barriers.get().RequestedAt.IsZero(), "nothing answered yet")

	barriers.request()
	req.Equal([]uint64{1}, leader.sent())
	requestedAt := barriers.pendingRequest().requestedAt
	req.False(requestedAt.IsZero())

	time.Sleep(time.Millisecond)
	handler.HandleReceive(readBarrierResponseMsg(1, 99), newRecordingChannel())

	req.Equal(ReadBarrier{Index: 99, RequestedAt: requestedAt}, barriers.get())
	req.Zero(barriers.pendingRequest().id, "answered, so nothing is outstanding")
}

// Test_ReadBarriers_LeavesAYoungRequestToFinish: re-asking every time a caller wants a barrier would replace
// each request before its answer arrived, and none would ever be accepted.
func Test_ReadBarriers_LeavesAYoungRequestToFinish(t *testing.T) {
	req := require.New(t)
	leader := &leaderStub{}
	barriers := newFollowerBarriers(leader)

	barriers.request()
	barriers.request()
	req.Equal([]uint64{1}, leader.sent())
}

// Test_ReadBarriers_ReplacesAnUnansweredRequestAndDropsItsLateAnswer: a leader that cannot answer sends
// nothing, so an old request is replaced. An answer to it arriving afterwards would pair its index with the
// newer request's time, which it cannot speak for.
func Test_ReadBarriers_ReplacesAnUnansweredRequestAndDropsItsLateAnswer(t *testing.T) {
	req := require.New(t)
	leader := &leaderStub{}
	barriers := newFollowerBarriers(leader)

	barriers.request()
	barriers.lock.Lock()
	barriers.pending.requestedAt = barriers.pending.requestedAt.Add(-readBarrierAnswerWait)
	barriers.lock.Unlock()

	barriers.request()
	req.Equal([]uint64{1, 2}, leader.sent())

	req.False(barriers.accept(1, 50), "the replaced request's answer is no longer wanted")
	req.True(barriers.get().RequestedAt.IsZero())

	req.True(barriers.accept(2, 60))
	req.Equal(uint64(60), barriers.get().Index)

	req.False(barriers.accept(2, 70), "an answer is accepted once")
	req.False(barriers.accept(0, 70), "nor is one naming no request")
	req.Equal(uint64(60), barriers.get().Index)
}

// Test_ReadBarriers_FailedSendDoesNotHoldUpTheNextRequest: a request that never left has no answer coming,
// so waiting it out would only delay the next.
func Test_ReadBarriers_FailedSendDoesNotHoldUpTheNextRequest(t *testing.T) {
	req := require.New(t)
	leader := &leaderStub{err: errors.New("no leader")}
	barriers := newFollowerBarriers(leader)

	barriers.request()
	req.Zero(barriers.pendingRequest().id)

	leader.lock.Lock()
	leader.err = nil
	leader.lock.Unlock()

	barriers.request()
	req.Equal([]uint64{2}, leader.sent())
}

// Test_ReadBarriers_LeaderTakesItsOwn: the leader answers itself the same way it answers a peer, without a
// round trip, and off the caller's goroutine.
func Test_ReadBarriers_LeaderTakesItsOwn(t *testing.T) {
	req := require.New(t)
	release := make(chan struct{})
	src := &fakeBarrierSource{leaderAndCaughtUp: true, appliedIndex: 31, barrierBlock: release}
	leader := &leaderStub{}
	barriers := newReadBarriers(func() bool { return true }, src.take, leader.send)

	barriers.request()
	req.True(barriers.get().RequestedAt.IsZero(), "the request returns before the barrier completes")

	close(release)
	req.Eventually(func() bool { return barriers.get().Index == 31 }, 2*time.Second, time.Millisecond)
	req.Empty(leader.sent(), "and nothing is sent")
}

// Test_ReadBarriers_LeaderThatCannotTakeOneAsksAgainAtOnce: a failed barrier leaves no answer coming.
func Test_ReadBarriers_LeaderThatCannotTakeOneAsksAgainAtOnce(t *testing.T) {
	req := require.New(t)
	src := &fakeBarrierSource{leaderAndCaughtUp: true, appliedIndex: 31, barrierErr: errors.New("no quorum")}
	barriers := newReadBarriers(func() bool { return true }, src.take, (&leaderStub{}).send)

	barriers.request()
	req.Eventually(func() bool { return barriers.pendingRequest().id == 0 }, 2*time.Second, time.Millisecond)
	req.True(barriers.get().RequestedAt.IsZero(), "a failed barrier is no evidence at all")

	barriers.request()
	req.Eventually(func() bool { return src.barrierCalls.Load() == 2 }, 2*time.Second, time.Millisecond)
}
