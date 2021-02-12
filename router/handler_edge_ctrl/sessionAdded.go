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
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/sync_strats"
	"github.com/openziti/edge/pb/edge_ctrl_pb"
	"github.com/openziti/edge/router/internal/fabric"
	"github.com/openziti/foundation/channel2"
	"sync"
	"time"
)

type sessionAddedHandler struct {
	sm          fabric.StateManager
	syncTracker *sessionSyncTracker
	reqChan     chan *sessionAddedWithState
	syncReady   chan []*edge_ctrl_pb.Session
	syncFail    chan error
}

func NewSessionAddedHandler(sm fabric.StateManager) *sessionAddedHandler {
	handler := &sessionAddedHandler{
		sm:        sm,
		reqChan:   make(chan *sessionAddedWithState, 100),
		syncReady: make(chan []*edge_ctrl_pb.Session, 0),
		syncFail:  make(chan error, 0),
	}

	go handler.startRecieveSync()
	go handler.startSyncApplier()
	go handler.startSyncFail()

	return handler
}

func (h *sessionAddedHandler) ContentType() int32 {
	return env.SessionAddedType
}

func (h *sessionAddedHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	go func() {
		req := &edge_ctrl_pb.SessionAdded{}
		if err := proto.Unmarshal(msg.Body, req); err == nil {
			if req.IsFullState {

				reqWithState := &sessionAddedWithState{
					SessionAdded: req,
				}

				if syncStrategyType, syncState, err := parseInstantSyncHeaders(msg); err == nil {
					reqWithState.SyncStrategyType = syncStrategyType
					reqWithState.InstantSyncState = syncState
				} else {
					pfxlog.Logger().WithField("strategy", syncStrategyType).WithField("msgContentType", msg.ContentType).WithError(err).Errorf("sync headers not present (old controller) or only partial present(error), treading as legacy: %v", err)
				}

				h.reqChan <- reqWithState
			} else {
				for _, session := range req.Sessions {
					h.sm.AddSession(session)
				}
			}
		} else {
			pfxlog.Logger().Panic("could not convert message as session added")
		}
	}()
}

func (h *sessionAddedHandler) startSyncApplier() {
	for sessions := range h.syncReady {
		for _, session := range sessions {
			h.sm.AddSession(session)
		}
		h.sm.RemoveMissingSessions(sessions)

		pfxlog.Logger().Infof("finished sychronizing sessions [count: %d]", len(sessions))
	}
}

func (h *sessionAddedHandler) startSyncFail() {

	for err := range h.syncFail {
		pfxlog.Logger().Errorf("failed to synchronize sessions, aborting: %v", err)
		h.syncTracker.Stop()
		h.syncTracker = nil
	}
}

func (h *sessionAddedHandler) legacySync(reqWithState *sessionAddedWithState) {
	pfxlog.Logger().Warn("using legacy sync logic some connections may be dropped")
	for _, session := range reqWithState.Sessions {
		h.sm.AddSession(session)
	}

	h.sm.RemoveMissingSessions(reqWithState.Sessions)
}

func (h *sessionAddedHandler) startRecieveSync() {
	for reqWithState := range h.reqChan {
		switch reqWithState.SyncStrategyType {
		case string(sync_strats.RouterSyncStrategyInstant):
			h.instantSync(reqWithState)
		case "":
			pfxlog.Logger().Warn("syncStrategy is not specifieid, old controller?")
			h.legacySync(reqWithState)
		default:
			pfxlog.Logger().Warnf("syncStrategy [%s] is not supported", reqWithState.SyncStrategyType)
			h.legacySync(reqWithState)
		}
	}
}

func (h *sessionAddedHandler) instantSync(reqWithState *sessionAddedWithState) {
	logger := pfxlog.Logger().WithField("strategy", reqWithState.SyncStrategyType)

	state := &sync_strats.InstantSyncState{}

	if reqWithState.InstantSyncState == nil {
		logger.Panic("syncState is empty, cannot continue")
	}

	if reqWithState.InstantSyncState.Id == "" {
		logger.Panic("syncState id is empty, cannot continue")
	}

	//if no id or the sync id is newer, reset
	if h.syncTracker == nil || h.syncTracker.syncId == "" || h.syncTracker.syncId < state.Id {
		logger.Warnf("new syncId [%s], resetting", state.Id)

		if h.syncTracker != nil {
			h.syncTracker.Stop()
		}

		h.syncTracker = newSessionSyncTracker(state.Id)

		h.syncTracker.StartDeadline(h.syncReady, h.syncFail, 20*time.Second)
	}

	//ignore older syncs
	if h.syncTracker.syncId > state.Id {
		logger.Warnf("older syncId [%s], ignoring", state.Id)
		return
	}

	h.syncTracker.Add(reqWithState)

}

type sessionSyncTracker struct {
	syncId           string
	syncLastRecieved bool
	reqsWithState    map[int]*sessionAddedWithState
	hasLast          bool
	lastSeq          int
	stop             chan struct{}
	deadline         sync.Once
	isDone           bool
}

func newSessionSyncTracker(id string) *sessionSyncTracker {
	return &sessionSyncTracker{
		syncId:        id,
		reqsWithState: map[int]*sessionAddedWithState{},
		stop:          make(chan struct{}, 0),
	}
}

func (tracker *sessionSyncTracker) Add(reqWithState *sessionAddedWithState) {
	tracker.reqsWithState[reqWithState.Sequence] = reqWithState

	if reqWithState.IsLast {
		tracker.hasLast = true
		tracker.lastSeq = reqWithState.Sequence
	}
}

func (tracker *sessionSyncTracker) Stop() {
	if tracker != nil {
		tracker.stop <- struct{}{}
	}
}

func (tracker *sessionSyncTracker) StartDeadline(syncReady chan []*edge_ctrl_pb.Session, syncFail chan error, timeout time.Duration) {
	tracker.deadline.Do(func() {
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			select {
			case <-tracker.stop:
				tracker.reqsWithState = nil
				syncFail <- nil
				return
			case <-ticker.C:
				if tracker.HasAll() {
					syncReady <- tracker.all()
					return
				}
			case <-time.After(timeout):
				tracker.reqsWithState = nil
				syncFail <- errors.New("timeout, did not receive all updates in time")
				return
			}
		}()
	})
}

func (tracker *sessionSyncTracker) HasAll() bool {
	if !tracker.hasLast {
		return false
	}

	hasAll := true

	for i := 0; i <= tracker.lastSeq; i++ {
		if req, ok := tracker.reqsWithState[i]; !ok && req == nil {
			hasAll = false
			break
		}
	}

	return hasAll
}

func (tracker *sessionSyncTracker) all() []*edge_ctrl_pb.Session {
	var result []*edge_ctrl_pb.Session
	for i := 0; i <= tracker.lastSeq; i++ {
		if req, ok := tracker.reqsWithState[i]; ok {
			for _, session := range req.Sessions {
				result = append(result, session)
			}
		} else {
			pfxlog.Logger().WithField("strategy", sync_strats.RouterSyncStrategyInstant).Error("all failed to have all update sequences")
		}
	}

	return result
}

type sessionAddedWithState struct {
	SyncStrategyType string
	*sync_strats.InstantSyncState
	*edge_ctrl_pb.SessionAdded
}

type sessionSyncResult struct {
	sessions []*sessionAddedWithState
}
