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

package model

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/storage/boltz"
	cmap "github.com/orcaman/concurrent-map/v2"
	"time"
)

type HeartbeatCollector struct {
	apiSessionLastAccessedAtMap cmap.ConcurrentMap[*HeartbeatStatus]
	updateInterval              time.Duration
	closeNotify                 chan struct{}
	isFlushing                  concurrenz.AtomicBoolean
	flushAction                 func(beats []*Heartbeat)
	batchSize                   int
}

type Heartbeat struct {
	ApiSessionId   string
	LastActivityAt time.Time
}

// NewHeartbeatCollector creates a HeartbeatCollector which is used to manage situations where an SDK is
// connecting to multiple Edge Routers and making API calls that all update their last updated at and trigger
// writes. The heartbeat collector aggregates all of those calls into a single write and acts as an in memory
// buffer for last update times.
func NewHeartbeatCollector(env Env, batchSize int, updateInterval time.Duration, action func([]*Heartbeat)) *HeartbeatCollector {
	collector := &HeartbeatCollector{
		apiSessionLastAccessedAtMap: cmap.New[*HeartbeatStatus](),
		updateInterval:              updateInterval,
		batchSize:                   batchSize,
		flushAction:                 action,
		closeNotify:                 make(chan struct{}, 0),
	}

	env.GetStores().ApiSession.AddListener(boltz.EventDelete, collector.onApiSessionDelete)

	collector.Start()

	return collector
}

type HeartbeatStatus struct {
	lastAccessedAt time.Time
	flushed        bool
}

func (self *HeartbeatCollector) Mark(apiSessionId string) {
	newStatus := &HeartbeatStatus{
		lastAccessedAt: time.Now().UTC(),
		flushed:        false,
	}
	self.apiSessionLastAccessedAtMap.Set(apiSessionId, newStatus)
}

// LastAccessedAt will return the last time an API Sessions was either connected to an Edge Router
// or made a REST API call and true. If no such action has happened or the API Session no longer exists
// nil and false will be returned.
func (self *HeartbeatCollector) LastAccessedAt(apiSessionId string) (*time.Time, bool) {
	if status, ok := self.apiSessionLastAccessedAtMap.Get(apiSessionId); ok {
		lastAccessedAt := status.lastAccessedAt
		return &lastAccessedAt, true
	}

	return nil, false
}

func (self *HeartbeatCollector) Start() {
	go self.run()
}

func (self *HeartbeatCollector) run() {
	for {
		select {
		case <-self.closeNotify:
			self.flush() //flush on stop
			return
		case <-time.After(self.updateInterval):
			self.flush()
		}
	}
}

func (self *HeartbeatCollector) Stop() {
	close(self.closeNotify)

}

func (self *HeartbeatCollector) flush() {
	if self.isFlushing.CompareAndSwap(false, true) {
		defer self.isFlushing.CompareAndSwap(true, false)
		pfxlog.Logger().Trace("flushing heartbeat collector")

		var beats []*Heartbeat

		self.apiSessionLastAccessedAtMap.IterCb(func(key string, status *HeartbeatStatus) {
			if len(beats) >= self.batchSize {
				self.flushAction(beats)
				beats = nil
			}

			if !status.flushed {
				status.flushed = true
				beats = append(beats, &Heartbeat{
					ApiSessionId:   key,
					LastActivityAt: status.lastAccessedAt,
				})
			}
		})

		if len(beats) > 0 {
			self.flushAction(beats)
		}

	} else {
		pfxlog.Logger().Warn("attempting to flush heartbeats from collector, a flush is already in progress, skipping")
	}
}

func (self *HeartbeatCollector) onApiSessionDelete(i ...interface{}) {
	if len(i) > 0 {
		apiSession := i[0].(*persistence.ApiSession)
		if apiSession != nil && apiSession.Id != "" {
			self.Remove(apiSession.Id)
		}
	}
}

func (self *HeartbeatCollector) Remove(id string) {
	self.apiSessionLastAccessedAtMap.Remove(id)
}
