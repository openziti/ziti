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
	"github.com/openziti/channel/v2"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/network"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sync"
	"sync/atomic"
)

// RouterSender represents a connection from an Edge Router to the controller. Used
// to asynchronously buffer and send messages to an Edge Router via Start() then Send()
type RouterSender struct {
	env.RouterState
	Id          string
	EdgeRouter  *model.EdgeRouter
	Router      *network.Router
	send        chan *channel.Message
	closeNotify chan struct{}
	running     atomic.Bool

	sync.Mutex
}

func newRouterSender(edgeRouter *model.EdgeRouter, router *network.Router, sendBufferSize int) *RouterSender {
	rtx := &RouterSender{
		Id:          eid.New(),
		EdgeRouter:  edgeRouter,
		Router:      router,
		send:        make(chan *channel.Message, sendBufferSize),
		closeNotify: make(chan struct{}),
		RouterState: env.NewLockingRouterStatus(),
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
	for {
		select {
		case <-rtx.closeNotify:
			return
		case msg := <-rtx.send:
			_ = rtx.Router.Control.Send(msg)
		}
	}
}

func (rtx *RouterSender) logger() *logrus.Entry {
	return pfxlog.Logger().
		WithField("routerTxId", rtx.Id).
		WithField("routerId", rtx.Router.Id).
		WithField("routerName", rtx.Router.Name).
		WithField("routerFingerprint", rtx.Router.Fingerprint).
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

func (m *routerTxMap) Remove(r *network.Router) {
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
	for entry := range m.internalMap.IterBuffered() {
		callback(entry.Val)
	}
}
