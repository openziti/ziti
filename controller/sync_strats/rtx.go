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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/channel/v3/protobufs"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

type syncRequest struct {
	currentIndex uint64
	duration     time.Duration
}

// RouterSender represents a connection from an Edge Router to the controller. Used
// to asynchronously buffer and send messages to an Edge Router via Start() then Send()
type RouterSender struct {
	env.RouterState
	Id               string
	EdgeRouter       *model.EdgeRouter
	Router           *model.Router
	send             chan *channel.Message
	closeNotify      chan struct{}
	routerDataModel  *common.RouterDataModel
	requestModelSync chan *edge_ctrl_pb.SubscribeToDataModelRequest
	modelChange      chan struct{}
	currentIndex     uint64
	syncRdmUntil     time.Time
	lastIndexSent    uint64
	running          atomic.Bool

	SupportsRouterModel bool

	sync.Mutex
}

func newRouterSender(edgeRouter *model.EdgeRouter, router *model.Router, sendBufferSize int, routerDataModel *common.RouterDataModel) *RouterSender {
	rtx := &RouterSender{
		Id:               eid.New(),
		EdgeRouter:       edgeRouter,
		Router:           router,
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

	idx, ok := rtx.routerDataModel.CurrentIndex()
	if !ok || idx <= rtx.lastIndexSent {
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
		logger.Info("data model subscription event received, but one already queued, attempting to clear")
	}

	select {
	case <-rtx.requestModelSync:
	default:
	}

	select {
	case rtx.requestModelSync <- request:
		return
	default:
		logger.Info("data model subscription event received, still can't queue, exiting")
	}
}

func (rtx *RouterSender) handleSyncRequest(req *edge_ctrl_pb.SubscribeToDataModelRequest) {
	if !req.Renew {
		rtx.currentIndex = req.CurrentIndex
	}

	if req.SubscriptionDurationSeconds < 10 {
		req.SubscriptionDurationSeconds = 10
	}

	if req.SubscriptionDurationSeconds > 360 {
		req.SubscriptionDurationSeconds = 360
	}

	rtx.syncRdmUntil = time.Now().Add(time.Duration(req.SubscriptionDurationSeconds) * time.Second)
	pfxlog.Logger().WithField("routerId", rtx.Router.Id).
		WithField("currentIndex", rtx.currentIndex).
		WithField("subscriptionDuration", rtx.syncRdmUntil.String()).
		Info("data model subscription started")

	rtx.handleModelChange()
}

func (rtx *RouterSender) handleModelChange() {
	if !rtx.syncRdmUntil.After(time.Now()) {
		return
	}

	logger := pfxlog.Logger().WithField("routerId", rtx.Router.Id).WithField("currentIndex", rtx.currentIndex)

	var events []*edge_ctrl_pb.DataState_ChangeSet
	var ok bool
	if rtx.currentIndex > 0 {
		events, ok = rtx.routerDataModel.ReplayFrom(rtx.currentIndex + 1)
	}

	var err error

	if ok {
		logger.Infof("event retrieval ok? %v, event count: %d for replay to router", ok, len(events))

		for _, curEvent := range events {
			if err = protobufs.MarshalTyped(curEvent).Send(rtx.Router.Control); err != nil {
				logger.WithError(err).
					WithField("eventIndex", curEvent.Index).
					WithField("evenType", reflect.TypeOf(curEvent).String()).
					WithField("eventIsSynthetic", curEvent.IsSynthetic).
					Error("could not send data state event")
				break
			}
			rtx.currentIndex = curEvent.Index
		}
	}

	if !ok || err != nil {
		logger.Info("could not send events for router sync, attempting full state")

		//full sync
		dataState := rtx.routerDataModel.GetDataState()

		if dataState == nil {
			return
		}

		if err = protobufs.MarshalTyped(dataState).Send(rtx.Router.Control); err != nil {
			logger.WithError(err).Error("failure sending full data state")
		} else {
			rtx.currentIndex = dataState.EndIndex
			logger.Infof("router synced data model to index %d", rtx.currentIndex)
		}
	}
}

func (rtx *RouterSender) logger() *logrus.Entry {
	return pfxlog.Logger().
		WithField("routerTxId", rtx.Id).
		WithField("routerId", rtx.Router.Id).
		WithField("routerName", rtx.Router.Name).
		WithField("routerFingerprint", *rtx.Router.Fingerprint).
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
