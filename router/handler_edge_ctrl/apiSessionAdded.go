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

package handler_edge_ctrl

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/sync_strats"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/router/fabric"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type apiSessionAddedHandler struct {
	control     channel2.Channel
	sm          fabric.StateManager
	syncTracker *apiSessionSyncTracker

	reqChan chan *apiSessionAddedWithState

	stop        chan struct{}
	stopped     concurrenz.AtomicBoolean
	trackerLock sync.Mutex
}

func NewApiSessionAddedHandler(sm fabric.StateManager, control channel2.Channel) *apiSessionAddedHandler {
	handler := &apiSessionAddedHandler{
		control: control,
		sm:      sm,
		reqChan: make(chan *apiSessionAddedWithState, 100),
		stop:    make(chan struct{}, 0),
	}

	go handler.startReceiveSync()

	control.AddCloseHandler(handler)

	return handler
}

func (h *apiSessionAddedHandler) HandleClose(_ channel2.Channel) {
	if h.stopped.CompareAndSwap(false, true) {
		close(h.stop)
	}
}

func (h *apiSessionAddedHandler) ContentType() int32 {
	return env.ApiSessionAddedType
}

func (h *apiSessionAddedHandler) HandleReceive(msg *channel2.Message, _ channel2.Channel) {
	go func() {
		req := &edge_ctrl_pb.ApiSessionAdded{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {
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

			for _, session := range req.ApiSessions {
				h.sm.AddApiSession(session)
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as api session added")
		}
	}()
}

func (h *apiSessionAddedHandler) applySync(tracker *apiSessionSyncTracker) {
	lastId := ""
	apiSessions := tracker.all()
	for _, apiSession := range apiSessions {
		if lastId == "" || apiSession.Id > lastId {
			lastId = apiSession.Id
		}
	}

	h.sm.RemoveMissingApiSessions(apiSessions, lastId)
	h.sm.MarkSyncStopped(tracker.syncId)

	tracker.isDone.Set(true)
	duration := tracker.endTime.Sub(tracker.startTime)
	logrus.Infof("finished sychronizing api sessions [count: %d, syncId: %s, duration: %v]", len(apiSessions), tracker.syncId, duration)
}

func (h *apiSessionAddedHandler) syncFailed(err error) {
	h.trackerLock.Lock()
	defer h.trackerLock.Unlock()

	logrus.WithError(err).Error("failed to synchronize api sessions")

	h.syncTracker.Stop()
	h.sm.MarkSyncStopped(h.syncTracker.syncId)

	h.syncTracker = nil

	resync := &edge_ctrl_pb.RequestClientReSync{
		Reason: fmt.Sprintf("error during api session sync: %v", err),
	}

	resyncProto, _ := proto.Marshal(resync)

	resyncMsg := channel2.NewMessage(env.RequestClientReSyncType, resyncProto)
	_ = h.control.Send(resyncMsg)
}

func (h *apiSessionAddedHandler) legacySync(reqWithState *apiSessionAddedWithState) {
	pfxlog.Logger().Warn("using legacy sync logic some connections may be dropped")
	for _, apiSession := range reqWithState.ApiSessions {
		h.sm.AddApiSession(apiSession)
	}

	h.sm.RemoveMissingApiSessions(reqWithState.ApiSessions, "")
}

func (h *apiSessionAddedHandler) startReceiveSync() {
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

func (h *apiSessionAddedHandler) instantSync(reqWithState *apiSessionAddedWithState) {
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
	if h.syncTracker == nil || h.syncTracker.syncId == "" || h.syncTracker.isDone.Get() || h.syncTracker.syncId < reqWithState.Id {

		if h.syncTracker == nil || h.syncTracker.syncId == "" {
			logger.Infof("first api session syncId [%s], starting", reqWithState.Id)
		} else if h.syncTracker.isDone.Get() {
			logger.Infof("api session syncId [%s], starting", reqWithState.Id)
		} else {
			logger.Infof("api session with newer syncId [old: %s, new: %s], aborting old, starting new", h.syncTracker.syncId, reqWithState.Id)
		}

		if h.syncTracker != nil {
			h.syncTracker.Stop()
		}

		h.syncTracker = newApiSessionSyncTracker(reqWithState.Id)
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

type apiSessionSyncTracker struct {
	syncId        string
	reqsWithState map[int]*apiSessionAddedWithState
	hasLast       bool
	lastSeq       int
	stop          chan struct{}
	isDone        concurrenz.AtomicBoolean
	lock          sync.Mutex
	startTime     time.Time
	endTime       time.Time
}

func newApiSessionSyncTracker(id string) *apiSessionSyncTracker {
	return &apiSessionSyncTracker{
		syncId:        id,
		reqsWithState: map[int]*apiSessionAddedWithState{},
		stop:          make(chan struct{}, 0),
		startTime:     time.Now(),
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
			for _, session := range reqWithState.ApiSessions {
				current.ApiSessions = append(current.ApiSessions, session)
			}
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

func (tracker *apiSessionSyncTracker) StartDeadline(timeout time.Duration, h *apiSessionAddedHandler) {
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
			for _, apiSession := range req.ApiSessions {
				result = append(result, apiSession)
			}
		} else {
			pfxlog.Logger().WithField("strategy", sync_strats.RouterSyncStrategyInstant).Error("all failed to have all update sequences")
		}
	}

	if req, ok := tracker.reqsWithState[-1]; ok {
		for _, apiSession := range req.ApiSessions {
			result = append(result, apiSession)
		}
	}

	return result
}

type apiSessionAddedWithState struct {
	SyncStrategyType string
	isPostSyncData   bool
	*sync_strats.InstantSyncState
	*edge_ctrl_pb.ApiSessionAdded
}

func parseInstantSyncHeaders(msg *channel2.Message) (string, *sync_strats.InstantSyncState, error) {
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
