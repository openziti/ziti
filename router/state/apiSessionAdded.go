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

package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/sync_strats"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// ApiSessionAddedHandler manages the reception and synchronization of API session
// additions from controllers, implementing sync strategies to handle both legacy
// sequential updates and modern chunked synchronization protocols.
//
// The handler maintains session consistency across controller failovers and
// network partitions, ensuring routers have accurate session state during
// multi-controller synchronization scenarios.
type ApiSessionAddedHandler struct {
	control     channel.Channel
	sm          Manager
	syncTracker *apiSessionSyncTracker

	reqChan chan *apiSessionAddedWithState

	stop        chan struct{}
	stopped     atomic.Bool
	trackerLock sync.Mutex
}

// NewApiSessionAddedHandler creates a new handler for API session addition events,
// establishing the necessary channels and goroutines for asynchronous session
// synchronization processing.
func NewApiSessionAddedHandler(sm Manager, binding channel.Binding) *ApiSessionAddedHandler {
	handler := &ApiSessionAddedHandler{
		control: binding.GetChannel(),
		sm:      sm,
		reqChan: make(chan *apiSessionAddedWithState, 100),
		stop:    make(chan struct{}),
	}

	go handler.startReceiveSync()

	binding.AddCloseHandler(handler)

	return handler
}

func (h *ApiSessionAddedHandler) HandleClose(_ channel.Channel) {
	if h.stopped.CompareAndSwap(false, true) {
		close(h.stop)
	}
}

func (h *ApiSessionAddedHandler) ContentType() int32 {
	return env.ApiSessionAddedType
}

func (h *ApiSessionAddedHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	go func() {
		req := &edge_ctrl_pb.ApiSessionAdded{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {
			for _, session := range req.ApiSessions {
				newApiSession := NewApiSessionTokenFromProtobuf(session, ch.Id())
				h.sm.AddLegacyApiSession(newApiSession)
			}

			if req.IsFullState {
				reqWithState := &apiSessionAddedWithState{
					ApiSessionAdded: req,
				}

				if syncStrategyType, syncState, err := parseInstantSyncHeaders(msg); err == nil {
					reqWithState.SyncStrategyType = syncStrategyType
					reqWithState.InstantSyncState = syncState
				} else {
					pfxlog.Logger().WithField("msgContentType", msg.ContentType).WithError(err).Errorf("sync headers not present (old controller) or only partial present(error), treating as legacy: %v", err)
				}

				h.reqChan <- reqWithState
			} else if h.sm.IsSyncInProgress() {
				reqWithState := &apiSessionAddedWithState{
					SyncStrategyType: string(sync_strats.RouterSyncStrategyInstant),
					ApiSessionAdded:  req,
					isPostSyncData:   true,
					InstantSyncState: &sync_strats.InstantSyncState{},
				}
				h.reqChan <- reqWithState
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as api session added")
		}
	}()
}

func (h *ApiSessionAddedHandler) applySync(tracker *apiSessionSyncTracker) {
	h.trackerLock.Lock()
	defer h.trackerLock.Unlock()

	lastId := ""
	apiSessions := tracker.all()
	apiSessionTokens := make([]*ApiSessionToken, 0, len(apiSessions))
	for _, apiSession := range apiSessions {
		if lastId == "" || apiSession.Id > lastId {
			lastId = apiSession.Id
		}
		apiSessionTokens = append(apiSessionTokens, NewApiSessionTokenFromProtobuf(apiSession, tracker.ctrlCh.Id()))
	}

	h.sm.RemoveMissingApiSessions(apiSessionTokens, lastId)
	h.sm.MarkSyncStopped(tracker.syncId)
	h.syncTracker = nil

	tracker.isDone.Store(true)
	duration := tracker.endTime.Sub(tracker.startTime)
	logrus.Infof("finished synchronizing api sessions [count: %d, syncId: %s, duration: %v]", len(apiSessions), tracker.syncId, duration)
}

func (h *ApiSessionAddedHandler) syncFailed(err error) {
	h.trackerLock.Lock()
	defer h.trackerLock.Unlock()

	// can be called twice, only notify the first time
	if h.syncTracker != nil {
		logrus.WithError(err).Error("failed to synchronize api sessions")

		h.syncTracker.Stop()
		h.sm.MarkSyncStopped(h.syncTracker.syncId)

		h.syncTracker = nil

		resync := &edge_ctrl_pb.RequestClientReSync{
			Reason: fmt.Sprintf("error during api session sync: %v", err),
		}
		if err := protobufs.MarshalTyped(resync).Send(h.control); err != nil {
			logrus.WithError(err).Error("failed to send request client re-sync message")
		}
	}
}

func (h *ApiSessionAddedHandler) legacySync(reqWithState *apiSessionAddedWithState) {
	pfxlog.Logger().Warn("using legacy sync logic some connections may be dropped")
	apiSessionTokens := make([]*ApiSessionToken, 0, len(reqWithState.ApiSessions))
	for _, apiSession := range reqWithState.ApiSessions {
		apiSessionToken := NewApiSessionTokenFromProtobuf(apiSession, h.control.Id())
		h.sm.AddLegacyApiSession(apiSessionToken)
		apiSessionTokens = append(apiSessionTokens, apiSessionToken)
	}

	h.sm.RemoveMissingApiSessions(apiSessionTokens, "")
}

func (h *ApiSessionAddedHandler) startReceiveSync() {
	for {
		select {
		case <-h.stop:
			return
		case reqWithState := <-h.reqChan:
			switch reqWithState.SyncStrategyType {
			case string(sync_strats.RouterSyncStrategyInstant):
				h.instantSync(reqWithState)
			case "":
				pfxlog.Logger().Warn("syncStrategy is not specified, old controller?")
				h.legacySync(reqWithState)
			default:
				pfxlog.Logger().Warnf("syncStrategy [%s] is not supported", reqWithState.SyncStrategyType)
				h.legacySync(reqWithState)
			}
		}
	}
}

func (h *ApiSessionAddedHandler) instantSync(reqWithState *apiSessionAddedWithState) {
	h.trackerLock.Lock()
	defer h.trackerLock.Unlock()

	logger := pfxlog.Logger().WithField("strategy", reqWithState.SyncStrategyType)

	if reqWithState.isPostSyncData {
		if h.syncTracker != nil {
			h.syncTracker.Add(reqWithState)
		}
		return
	}

	if reqWithState.InstantSyncState == nil {
		logger.Panic("syncState is empty, cannot continue")
	}

	if reqWithState.InstantSyncState.Id == "" {
		logger.Panic("syncState id is empty, cannot continue")
	}

	//if no id or the sync id is newer, reset
	if h.syncTracker == nil || h.syncTracker.syncId == "" || h.syncTracker.isDone.Load() || h.syncTracker.syncId < reqWithState.Id {

		if h.syncTracker == nil || h.syncTracker.syncId == "" {
			logger.Infof("first api session syncId [%s], starting", reqWithState.Id)
		} else if h.syncTracker.isDone.Load() {
			logger.Infof("api session syncId [%s], starting", reqWithState.Id)
		} else {
			logger.Infof("api session with newer syncId [old: %s, new: %s], aborting old, starting new", h.syncTracker.syncId, reqWithState.Id)
		}

		if h.syncTracker != nil {
			h.syncTracker.Stop()
		}

		h.syncTracker = newApiSessionSyncTracker(reqWithState.Id, h.control)
		h.sm.MarkSyncInProgress(h.syncTracker.syncId)
		go h.syncTracker.StartDeadline(20*time.Second, h)
	}

	//ignore older syncs
	if h.syncTracker.syncId > reqWithState.Id {
		logger.Warnf("older syncId [%s], ignoring", reqWithState.Id)
		return
	}

	h.syncTracker.Add(reqWithState)
}

// apiSessionSyncTracker manages the collection and ordering of chunked API session
// synchronization messages, ensuring all sequence numbers are received before
// applying the complete session state update.
//
// The tracker handles out-of-order message delivery and provides timeout
// mechanisms to prevent indefinite waiting for missing chunks.
type apiSessionSyncTracker struct {
	syncId        string
	reqsWithState map[int]*apiSessionAddedWithState
	hasLast       bool
	lastSeq       int
	stop          chan struct{}
	isDone        atomic.Bool
	lock          sync.Mutex
	startTime     time.Time
	endTime       time.Time
	ctrlCh        channel.Channel
}

func newApiSessionSyncTracker(id string, ctrlCh channel.Channel) *apiSessionSyncTracker {
	return &apiSessionSyncTracker{
		syncId:        id,
		reqsWithState: map[int]*apiSessionAddedWithState{},
		stop:          make(chan struct{}),
		startTime:     time.Now(),
		ctrlCh:        ctrlCh,
	}
}

func (tracker *apiSessionSyncTracker) Clear() {
	tracker.lock.Lock()
	defer tracker.lock.Unlock()
	tracker.reqsWithState = map[int]*apiSessionAddedWithState{}
}

func (tracker *apiSessionSyncTracker) Add(reqWithState *apiSessionAddedWithState) {
	tracker.lock.Lock()
	defer tracker.lock.Unlock()

	if reqWithState.isPostSyncData {
		current := tracker.reqsWithState[-1]
		if current != nil {
			current.ApiSessions = append(current.ApiSessions, reqWithState.ApiSessions...)
		} else {
			tracker.reqsWithState[-1] = reqWithState
		}
	} else {
		tracker.reqsWithState[reqWithState.Sequence] = reqWithState
		logrus.Infof("received api session sync chunk %v, isLast=%v", reqWithState.Sequence, reqWithState.IsLast)
		if reqWithState.IsLast {
			tracker.hasLast = true
			tracker.lastSeq = reqWithState.Sequence
			tracker.endTime = time.Now()
		}
	}
}

func (tracker *apiSessionSyncTracker) Stop() {
	if tracker != nil && tracker.stop != nil {
		close(tracker.stop)
		tracker.stop = nil
	}
}

func (tracker *apiSessionSyncTracker) StartDeadline(timeout time.Duration, h *ApiSessionAddedHandler) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	deadlineTimer := time.NewTimer(timeout)
	defer deadlineTimer.Stop()

	for {
		select {
		case <-tracker.stop:
			tracker.Clear()
			return
		case <-ticker.C:
			if tracker.HasAll() {
				h.applySync(tracker)
				return
			}
		case <-deadlineTimer.C:
			tracker.Clear()
			h.syncFailed(errors.New("timeout, did not receive all updates in time"))
			return
		}
	}
}

func (tracker *apiSessionSyncTracker) HasAll() bool {
	tracker.lock.Lock()
	defer tracker.lock.Unlock()

	if !tracker.hasLast {
		return false
	}

	for i := 0; i <= tracker.lastSeq; i++ {
		if req, ok := tracker.reqsWithState[i]; !ok && req == nil {
			return false
		}
	}

	return true
}

func (tracker *apiSessionSyncTracker) all() []*edge_ctrl_pb.ApiSession {
	tracker.lock.Lock()
	defer tracker.lock.Unlock()

	var result []*edge_ctrl_pb.ApiSession
	for i := 0; i <= tracker.lastSeq; i++ {
		if req, ok := tracker.reqsWithState[i]; ok {
			result = append(result, req.ApiSessions...)
		} else {
			pfxlog.Logger().WithField("strategy", sync_strats.RouterSyncStrategyInstant).Error("all failed to have all update sequences")
		}
	}

	if req, ok := tracker.reqsWithState[-1]; ok {
		result = append(result, req.ApiSessions...)
	}

	return result
}

// apiSessionAddedWithState combines API session data with synchronization
// metadata, enabling the router to distinguish between different sync strategies
// and handle both initial sync data and post-sync incremental updates.
type apiSessionAddedWithState struct {
	SyncStrategyType string
	isPostSyncData   bool
	*sync_strats.InstantSyncState
	*edge_ctrl_pb.ApiSessionAdded
}

func parseInstantSyncHeaders(msg *channel.Message) (string, *sync_strats.InstantSyncState, error) {
	if syncStrategyType, ok := msg.Headers[env.SyncStrategyTypeHeader]; ok {
		if syncStrategyState, ok := msg.Headers[env.SyncStrategyStateHeader]; ok {
			state := &sync_strats.InstantSyncState{}
			if err := json.Unmarshal(syncStrategyState, state); err == nil {
				return string(syncStrategyType), state, nil
			} else {
				pfxlog.Logger().WithField("strategy", syncStrategyType).WithField("msgContentType", msg.ContentType).Panicf("could not parse sync state [%s], error: %v", syncStrategyState, err)
			}

		} else {
			return "", nil, errors.New("received sync message with a strategy type header, but no state")
		}
	}
	return "", nil, errors.New("received sync message with no strategy type header")
}
