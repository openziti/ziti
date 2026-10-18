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
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/channel/v5/protobufs"
	"github.com/openziti/ziti/v2/common/pb/cmd_pb"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

const (
	// readBarrierTimeout bounds the raft barrier a leader takes to answer, which commits through quorum.
	readBarrierTimeout = 5 * time.Second
	// readBarrierResponseTimeout bounds the leader's reply send.
	readBarrierResponseTimeout = 5 * time.Second
	// readBarrierAnswerWait is how long a request is left to be answered before another may replace it.
	// A leader that cannot answer sends nothing, so this is also how a refusal is noticed.
	readBarrierAnswerWait = readBarrierTimeout + readBarrierResponseTimeout
)

// ReadBarrier is a store index, paired with the local time the request for it was made. Once this
// controller's store has applied up to Index, it has applied everything the cluster had committed at
// RequestedAt. The zero value is a barrier never answered, which covers nothing.
type ReadBarrier struct {
	Index       uint64
	RequestedAt time.Time
}

// readBarriers tracks this controller's read barrier requests: the latest answered, and at most one awaiting
// an answer. Requests carry an id and only the outstanding one's answer is accepted, so a late answer to a
// replaced request cannot pair its index with another request's time.
type readBarriers struct {
	isLeader     func() bool
	take         func() (uint64, error)
	sendToLeader func(request *cmd_pb.ReadBarrierRequest) error

	lock    sync.Mutex
	lastId  uint64
	pending pendingReadBarrier
	latest  ReadBarrier
}

type pendingReadBarrier struct {
	id          uint64
	requestedAt time.Time
}

func newReadBarriers(isLeader func() bool, take func() (uint64, error), sendToLeader func(*cmd_pb.ReadBarrierRequest) error) *readBarriers {
	return &readBarriers{
		isLeader:     isLeader,
		take:         take,
		sendToLeader: sendToLeader,
	}
}

// request starts a read barrier request unless one made within readBarrierAnswerWait is still unanswered.
// Returns without waiting: the leader answers its own on a new goroutine, and anyone else sends to the leader
// and is answered through accept.
func (self *readBarriers) request() {
	id, started := self.begin()
	if !started {
		return
	}

	if self.isLeader() {
		go self.answerLocally(id)
		return
	}

	if err := self.sendToLeader(&cmd_pb.ReadBarrierRequest{RequestId: id}); err != nil {
		pfxlog.Logger().WithError(err).Debug("unable to request read barrier from leader")
		self.abandon(id)
	}
}

func (self *readBarriers) begin() (uint64, bool) {
	self.lock.Lock()
	defer self.lock.Unlock()

	// The time is taken under the lock so ids and times advance together, and a later id never carries
	// an earlier time.
	now := time.Now()
	if self.pending.id != 0 && now.Sub(self.pending.requestedAt) < readBarrierAnswerWait {
		return 0, false
	}

	self.lastId++
	self.pending = pendingReadBarrier{id: self.lastId, requestedAt: now}
	return self.lastId, true
}

func (self *readBarriers) answerLocally(id uint64) {
	index, err := self.take()
	if err != nil {
		pfxlog.Logger().WithError(err).Debug("unable to take read barrier")
		self.abandon(id)
		return
	}
	self.accept(id, index)
}

// abandon clears the outstanding request if it is still id, so the next request goes out without waiting
// for this one's answer to time out.
func (self *readBarriers) abandon(id uint64) {
	self.lock.Lock()
	defer self.lock.Unlock()
	if self.pending.id == id {
		self.pending = pendingReadBarrier{}
	}
}

// accept records index as the answer to request id, if that is the outstanding request, reporting whether
// it did.
func (self *readBarriers) accept(id uint64, index uint64) bool {
	self.lock.Lock()
	defer self.lock.Unlock()
	if id == 0 || self.pending.id != id {
		return false
	}
	self.latest = ReadBarrier{Index: index, RequestedAt: self.pending.requestedAt}
	self.pending = pendingReadBarrier{}
	return true
}

func (self *readBarriers) get() ReadBarrier {
	self.lock.Lock()
	defer self.lock.Unlock()
	return self.latest
}

// RequestReadBarrier asks the leader for a read barrier and returns without waiting; GetReadBarrier reports
// the answer once it arrives. A request still awaiting its answer is left to finish rather than replaced,
// and one the leader cannot answer is dropped, so callers that still need a barrier ask again later.
func (self *Controller) RequestReadBarrier() {
	self.readBarriers.request()
}

// GetReadBarrier returns the latest answered read barrier. Its Index is in the measure GetAppliedIndex
// reports.
func (self *Controller) GetReadBarrier() ReadBarrier {
	return self.readBarriers.get()
}

// ReadBarrierHandlers returns the peer channel handlers that answer read barrier requests and accept the
// answers to this controller's own.
func (self *Controller) ReadBarrierHandlers() []channel.ContentTypeReceiver {
	return []channel.ContentTypeReceiver{
		&readBarrierRequestHandler{take: self.readBarriers.take},
		&readBarrierResponseHandler{barriers: self.readBarriers},
	}
}

// readBarrierSource is what taking a read barrier needs from raft.
type readBarrierSource interface {
	IsLeaderAndCaughtUp() bool
	GetAppliedIndex() uint64
	barrier(timeout time.Duration) error
}

// takeReadBarrier runs a raft barrier and returns the store index it established. Fails unless src is the
// leader and has caught up with its term, and when the barrier cannot commit through quorum.
func takeReadBarrier(src readBarrierSource) (uint64, error) {
	if !src.IsLeaderAndCaughtUp() {
		return 0, raft.ErrNotLeader
	}
	if err := src.barrier(readBarrierTimeout); err != nil {
		return 0, err
	}
	// The store's tracker rather than raft's applied index, which also counts barrier and no-op entries
	// the store never sees, and so may never be reached by the requester.
	return src.GetAppliedIndex(), nil
}

func (self *Controller) barrier(timeout time.Duration) error {
	return self.Raft.Barrier(timeout).Error()
}

func (self *Controller) sendReadBarrierRequest(request *cmd_pb.ReadBarrierRequest) error {
	leaderAddr := self.GetLeaderAddr()
	if leaderAddr == "" {
		return errors.New("no leader")
	}

	peer := self.Mesh.GetPeer(raft.ServerAddress(leaderAddr))
	if peer == nil {
		return errors.Errorf("not connected to leader at %s", leaderAddr)
	}

	queued, err := peer.Channel.TrySend(protobufs.MarshalTyped(request).ToSendable())
	if err != nil {
		return err
	}
	if !queued {
		return errors.Errorf("send queue to leader at %s is full", leaderAddr)
	}
	return nil
}

// readBarrierRequestHandler answers read barrier requests from peers. A node that cannot take the barrier
// sends nothing; the requester notices by the answer not arriving.
type readBarrierRequestHandler struct {
	take func() (uint64, error)
}

func (self *readBarrierRequestHandler) ContentType() int32 {
	return int32(cmd_pb.ContentType_ReadBarrierRequestType)
}

// HandleReceive answers on a new goroutine. The barrier waits on quorum, and the receive goroutine carries
// every other message on this channel, heartbeats included.
func (self *readBarrierRequestHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	request := &cmd_pb.ReadBarrierRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal read barrier request")
		return
	}
	go self.answer(request.RequestId, ch)
}

func (self *readBarrierRequestHandler) answer(requestId uint64, ch channel.Channel) {
	log := pfxlog.Logger().WithField("peerId", ch.Id()).WithField("requestId", requestId)

	index, err := self.take()
	if err != nil {
		log.WithError(err).Debug("unable to take read barrier for peer")
		return
	}

	response := &cmd_pb.ReadBarrierResponse{RequestId: requestId, Index: index}
	if err = protobufs.MarshalTyped(response).WithTimeout(readBarrierResponseTimeout).Send(ch); err != nil {
		log.WithError(err).Debug("unable to send read barrier response")
	}
}

// readBarrierResponseHandler accepts answers to this controller's read barrier requests.
type readBarrierResponseHandler struct {
	barriers *readBarriers
}

func (self *readBarrierResponseHandler) ContentType() int32 {
	return int32(cmd_pb.ContentType_ReadBarrierResponseType)
}

func (self *readBarrierResponseHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	response := &cmd_pb.ReadBarrierResponse{}
	if err := proto.Unmarshal(msg.Body, response); err != nil {
		pfxlog.Logger().WithError(err).Error("unable to unmarshal read barrier response")
		return
	}

	if !self.barriers.accept(response.RequestId, response.Index) {
		pfxlog.Logger().WithField("peerId", ch.Id()).WithField("requestId", response.RequestId).
			Debug("read barrier response does not answer the outstanding request, ignoring")
	}
}
