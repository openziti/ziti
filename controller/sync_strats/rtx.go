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

package sync_strats

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/eid"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/sirupsen/logrus"
	"sync"
)

// RouterSender represents a connection from an Edge Router to the controller. Used
// to asynchronously buffer and send messages to an Edge Router via Start() then Send()
type RouterSender struct {
	env.RouterState
	Id         string
	EdgeRouter *model.EdgeRouter
	Router     *network.Router
	send       chan *channel2.Message
	stop       chan interface{}
	running    concurrenz.AtomicBoolean
	stopping   concurrenz.AtomicBoolean

	sync.Mutex
}

func newRouterSender(edgeRouter *model.EdgeRouter, router *network.Router, sendBufferSize int) *RouterSender {
	return &RouterSender{
		Id:          eid.New(),
		EdgeRouter:  edgeRouter,
		Router:      router,
		send:        make(chan *channel2.Message, sendBufferSize),
		stop:        make(chan interface{}, 0),
		running:     concurrenz.AtomicBoolean(0),
		stopping:    concurrenz.AtomicBoolean(0),
		RouterState: env.NewLockingRouterStatus(),
	}
}

func (rtx *RouterSender) GetState() env.RouterStateValues {
	if rtx == nil {
		return env.NewRouterStatusValues()
	}

	return rtx.Values()
}

func (rtx *RouterSender) Start() {
	if rtx.running.CompareAndSwap(false, true) {
		go rtx.run()
	}
}

func (rtx *RouterSender) Stop() {
	if rtx.stopping.CompareAndSwap(false, true) {
		go func() {
			rtx.stop <- struct{}{}
		}()
	}
}

func (rtx *RouterSender) run() {
	for {
		select {
		case <-rtx.stop:
			rtx.running.Set(false)
			rtx.stopping.Set(false)
			return
		case msg := <-rtx.send:
			if !rtx.Router.Control.IsClosed() {
				_ = rtx.Router.Control.Send(msg)
			}
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

func (rtx *RouterSender) Send(msg *channel2.Message) {
	rtx.send <- msg
}

// Map used make working with internal RouterSender easier as sync.Map accepts and returns interface{}
type routerTxMap struct {
	internalMap *sync.Map //id -> RouterSender
}

func (m *routerTxMap) Add(id string, routerMessageTxer *RouterSender) {
	m.internalMap.Store(id, routerMessageTxer)
	routerMessageTxer.Start()
}

func (m *routerTxMap) Get(id string) *RouterSender {
	val, found := m.internalMap.Load(id)
	if !found {
		return nil
	}
	return val.(*RouterSender)
}

func (m *routerTxMap) GetState(id string) env.RouterStateValues {
	rtx := m.Get(id)
	return rtx.GetState()
}

func (m *routerTxMap) Remove(id string) {
	entry := m.Get(id)
	if entry != nil {
		entry.Stop()
		m.internalMap.Delete(id)
	}
}

func (m *routerTxMap) Range(f func(entries *RouterSender) bool) {
	m.internalMap.Range(func(edgeRouterId, value interface{}) bool {
		if rtx, ok := value.(*RouterSender); ok {
			return f(rtx)
		}
		pfxlog.Logger().Panic("could not convert edge router entry")
		return false
	})
}

// Helper to generate channel2.ReceiveHandler instances from a contentType and function
func newReceiveHandlerFunc(contentType int32, handler func(m *channel2.Message, ch channel2.Channel)) channel2.ReceiveHandler {
	return receiveHandlerFunc{
		contentType: contentType,
		handler:     handler,
	}
}

var _ channel2.ReceiveHandler = &receiveHandlerFunc{}

type receiveHandlerFunc struct {
	contentType int32
	handler     func(m *channel2.Message, ch channel2.Channel)
}

func (r receiveHandlerFunc) ContentType() int32 {
	return r.contentType
}

func (r receiveHandlerFunc) HandleReceive(m *channel2.Message, ch channel2.Channel) {
	r.handler(m, ch)
}
