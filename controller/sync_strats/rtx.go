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

package sync_strats

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

// RouterSender represents a connection from an Edge Router to the controller. Used
// to asynchronously buffer and send messages to an Edge Router via Start() then Send()
type RouterSender struct {
	env.RouterState
	Id               string
	EdgeRouter       *model.EdgeRouter
	Router           *model.Router
	ae               *env.AppEnv
	send             chan *channel.Message
	closeNotify      chan struct{}
	routerDataModel  *common.RouterDataModelSender
	requestModelSync chan *edge_ctrl_pb.SubscribeToDataModelRequest
	modelChange      chan struct{}
	currentIndex     uint64
	syncRdmUntil     time.Time
	lastIndexSent    uint64
	running          atomic.Bool
	timelineId       string
	subscriptionId   string

	SupportsRouterModel bool

	sync.Mutex
}

func newRouterSender(ae *env.AppEnv, edgeRouter *model.EdgeRouter, router *model.Router, sendBufferSize int, routerDataModel *common.RouterDataModelSender) *RouterSender {
	rtx := &RouterSender{
		Id:               eid.New(),
		EdgeRouter:       edgeRouter,
		Router:           router,
		ae:               ae,
		send:             make(chan *channel.Message, sendBufferSize),
		requestModelSync: make(chan *edge_ctrl_pb.SubscribeToDataModelRequest, 1),
		modelChange:      make(chan struct{}, 1),
		closeNotify:      make(chan struct{}),
		RouterState:      env.NewLockingRouterStatus(),
		routerDataModel:  routerDataModel,
	}
	rtx.running.Store(true)

	go rtx.run()

	return rtx
}

func (rtx *RouterSender) GetState() env.RouterStateValues {
	if rtx == nil {
		return env.NewRouterStatusValues()
	}

	return rtx.Values()
}

func (rtx *RouterSender) Stop() {
	if rtx.running.CompareAndSwap(true, false) {
		close(rtx.closeNotify)
	}
}

func (rtx *RouterSender) run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rtx.closeNotify:
			return
		case msg := <-rtx.send:
			_ = rtx.Router.Control.Send(msg)
		case req := <-rtx.requestModelSync:
			rtx.handleSyncRequest(req)
		case <-rtx.modelChange:
			rtx.handleModelChange()
		case <-ticker.C:
			rtx.sendRouterDataModelIndex()
		}
	}
}

func (rtx *RouterSender) notifyOfModelChange() {
	select {
	case rtx.modelChange <- struct{}{}:
	default:
	}
}

func (rtx *RouterSender) sendRouterDataModelIndex() {
	if rtx.routerDataModel == nil {
		return
	}

	idx := rtx.routerDataModel.CurrentIndex()
	if idx <= rtx.lastIndexSent {
		return
	}

	msg := channel.NewMessage(int32(edge_ctrl_pb.ContentType_CurrentIndexMessageType), nil)
	msg.PutUint64Header(int32(edge_ctrl_pb.Header_RouterDataModelIndex), idx)

	if err := rtx.Router.Control.Send(msg); err != nil {
		logger := pfxlog.Logger().WithField("routerId", rtx.Router.Id)
		logger.WithError(err).Error("could not send current router data model index")
	} else {
		rtx.lastIndexSent = idx
	}
}

func (rtx *RouterSender) subscribe(request *edge_ctrl_pb.SubscribeToDataModelRequest) {
	logger := pfxlog.Logger().WithField("routerId", rtx.Router.Id)
	select {
	case rtx.requestModelSync <- request:
		return
	default:
		// already a request queued, let's try to clear it
		logger.Debug("data model subscription event received, but one already queued, attempting to clear")
	}

	select {
	case <-rtx.requestModelSync:
	default:
	}

	select {
	case rtx.requestModelSync <- request:
		return
	default:
		logger.Debug("data model subscription event received, still can't queue, exiting")
	}
}

func (rtx *RouterSender) handleSyncRequest(req *edge_ctrl_pb.SubscribeToDataModelRequest) {
	if !req.Renew || req.SubscriptionId != rtx.subscriptionId {
		rtx.currentIndex = req.CurrentIndex
	}

	if req.SubscriptionDurationSeconds < 10 {
		req.SubscriptionDurationSeconds = 10
	}

	if req.SubscriptionDurationSeconds > 360 {
		req.SubscriptionDurationSeconds = 360
	}

	rtx.timelineId = req.TimelineId
	rtx.subscriptionId = req.SubscriptionId

	rtx.syncRdmUntil = time.Now().Add(time.Duration(req.SubscriptionDurationSeconds) * time.Second)
	pfxlog.Logger().WithField("routerId", rtx.Router.Id).
		WithField("routerName", rtx.Router.Name).
		WithField("requestedIndex", req.CurrentIndex).
		WithField("currentIndex", rtx.currentIndex).
		WithField("renew", req.Renew).
		WithField("subscriptionDuration", rtx.syncRdmUntil.String()).
		WithField("timelineId", rtx.timelineId).
		WithField("subscriptionId", rtx.subscriptionId).
		Info("data model subscription started")

	rtx.handleModelChange()
}

func (rtx *RouterSender) handleModelChange() {
	if !rtx.syncRdmUntil.After(time.Now()) {
		return
	}

	logger := pfxlog.Logger().WithField("routerId", rtx.Router.Id).
		WithField("currentIndex", rtx.currentIndex).
		WithField("routerTimelineId", rtx.timelineId).
		WithField("ctrlTimelineId", rtx.routerDataModel.GetTimelineId())

	var events []*edge_ctrl_pb.DataState_ChangeSet
	replayResult := common.ReplayResultFullSyncRequired

	if rtx.currentIndex > 0 && rtx.timelineId == rtx.routerDataModel.GetTimelineId() {
		events, replayResult = rtx.routerDataModel.ReplayFrom(rtx.currentIndex + 1)
	}

	var err error

	logger.Debugf("event retrieval ok? %s, event count: %d for replay to router", replayResult.String(), len(events))

	if replayResult == common.ReplayResultSuccess {
		for _, curEvent := range events {
			var msg *channel.Message
			if msg, err = rtx.marshal(curEvent); err == nil {
				err = rtx.Router.Control.Send(msg)
			}

			if err != nil {
				logger.WithError(err).
					WithField("eventIndex", curEvent.Index).
					WithField("eventType", reflect.TypeOf(curEvent).String()).
					WithField("synthetic", curEvent.IsSynthetic).
					Error("could not send data state event")
				break
			}
			logger.
				WithField("eventIndex", curEvent.Index).
				WithField("eventType", reflect.TypeOf(curEvent).String()).
				WithField("synthetic", curEvent.IsSynthetic).
				Info("data state event sent to router")
			rtx.currentIndex = curEvent.Index
		}
	}

	var fullSync bool

	if err != nil {
		fullSync = true
		logger.Info("could not send events for router sync, attempting full state sync")
	} else if replayResult == common.ReplayResultFullSyncRequired {
		fullSync = true
		logger.Info("required sync events not found in event cache, attempting full state sync")
	} else if rtx.ae.GetCommandDispatcher().IsLeader() && replayResult == common.ReplayResultRequestFromFuture {
		fullSync = true
		logger.Info("index is in future, and this node is the leader, attempting full state sync")
	}

	if fullSync {
		//full sync
		dataState := rtx.routerDataModel.GetDataState()

		if dataState == nil {
			return
		}

		var msg *channel.Message
		if msg, err = rtx.marshal(dataState); err == nil {
			err = rtx.Router.Control.Send(msg)
		}

		if err != nil {
			logger.WithError(err).Error("failure sending full data state")
		} else {
			rtx.currentIndex = dataState.EndIndex
			rtx.timelineId = dataState.TimelineId
			logger.Infof("router synced data model to index %d with on timeline: %s", rtx.currentIndex, rtx.timelineId)
		}
	}
}

func (rtx *RouterSender) marshal(message protobufs.TypedMessage) (*channel.Message, error) {
	b, err := proto.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %v (%w)", reflect.TypeOf(message), err)
	}
	result := channel.NewMessage(message.GetContentType(), b)
	if rtx.subscriptionId != "" {
		result.PutStringHeader(int32(edge_ctrl_pb.Header_RouterDataModelSubscriptionId), rtx.subscriptionId)
	}
	return result, nil
}

func (rtx *RouterSender) logger() *logrus.Entry {
	return pfxlog.Logger().
		WithField("routerTxId", rtx.Id).
		WithField("routerId", rtx.Router.Id).
		WithField("routerName", rtx.Router.Name).
		WithField("routerFingerprint", stringz.OrEmpty(rtx.Router.Fingerprint)).
		WithField("routerChannelIsOpen", !rtx.Router.Control.IsClosed())
}

func (rtx *RouterSender) Send(msg *channel.Message) error {
	if !rtx.running.Load() {
		rtx.logger().Errorf("cannot send to router [%s], rtx'er is stopped", rtx.Router.Id)
		return errors.Errorf("cannot send to router [%s], rtx'er is stopped", rtx.Router.Id)
	}

	if rtx.Router.Control.IsClosed() {
		rtx.logger().Errorf("cannot send to router [%s], rtx's channel is closed", rtx.Router.Id)
		rtx.Stop()
		return errors.Errorf("cannot send to router [%s], rtx's channel is closed", rtx.Router.Id)
	}

	select {
	case rtx.send <- msg:
	case <-rtx.closeNotify:
	}

	return nil
}

// Map used make working with internal RouterSender easier as sync.Map accepts and returns interface{}
type routerTxMap struct {
	internalMap cmap.ConcurrentMap[string, *RouterSender] //id -> RouterSender
}

func (m *routerTxMap) Add(id string, routerMessageTxer *RouterSender) {
	m.internalMap.Set(id, routerMessageTxer)
}

func (m *routerTxMap) Get(id string) *RouterSender {
	val, found := m.internalMap.Get(id)
	if !found {
		return nil
	}
	return val
}

func (m *routerTxMap) GetState(id string) env.RouterStateValues {
	rtx := m.Get(id)
	return rtx.GetState()
}

func (m *routerTxMap) Remove(r *model.Router) {
	var rtx *RouterSender
	m.internalMap.RemoveCb(r.Id, func(key string, v *RouterSender, exists bool) bool {
		if !exists {
			return false
		}
		if v.Router == r {
			rtx = v
			return true
		}
		return false
	})

	if rtx != nil {
		rtx.Stop()
	}
}

// Range creates a snapshot of the rtx's to loop over and will execute callback
// with each rtx.
func (m *routerTxMap) Range(callback func(entries *RouterSender)) {
	var routerSenders []*RouterSender
	m.internalMap.IterCb(func(_ string, v *RouterSender) {
		routerSenders = append(routerSenders, v)
	})
	for _, routerSender := range routerSenders {
		callback(routerSender)
	}
}
