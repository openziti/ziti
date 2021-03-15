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
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/concurrenz"
	cmap "github.com/orcaman/concurrent-map"
	"time"
)

type HeartbeatCollector struct {
	apiSessionLastUpdate cmap.ConcurrentMap //map[apiSessionId string] => *HeartbeatStatus
	updateInterval       time.Duration
	closeNotify          chan struct{}
	isFlushing           concurrenz.AtomicBoolean
	flushAction          func(beats []*Heartbeat)
	batchSize            int
}

type Heartbeat struct {
	ApiSessionId   string
	LastActivityAt time.Time
}

// Creates a new HeartbeatCollector which is used to manage situations where an SDK is connecting to multiiple
// Edge Routers and making API calls that all update their last updated at and trigger a write. The heartbeat
// collector aggregates all of those calls into a single write and acts as an in memory buffer for last update
// times.
func NewHeartbeatCollector(env Env, batchSize int, updateInterval time.Duration, action func([]*Heartbeat)) *HeartbeatCollector {
	collector := &HeartbeatCollector{
		apiSessionLastUpdate: cmap.New(),
		updateInterval:       updateInterval,
		batchSize:            batchSize,
		flushAction:          action,
		closeNotify:          make(chan struct{}, 0),
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
	self.apiSessionLastUpdate.Set(apiSessionId, newStatus)
}

func (self *HeartbeatCollector) LastAccessedAt(apiSessionId string) (time.Time, bool) {
	if val, ok := self.apiSessionLastUpdate.Get(apiSessionId); ok {
		if status, ok := val.(*HeartbeatStatus); ok {
			return status.lastAccessedAt, true
		}
	}

	return time.Time{}, false
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
		pfxlog.Logger().Trace("flushing heartbeat collector")

		var beats []*Heartbeat

		self.apiSessionLastUpdate.IterCb(func(key string, v interface{}) {
			if len(beats) >= self.batchSize {
				self.flushAction(beats)
				beats = nil
			}

			if status, ok := v.(*HeartbeatStatus); ok {
				if !status.flushed {
					status.flushed = true
					beats = append(beats, &Heartbeat{
						ApiSessionId:   key,
						LastActivityAt: status.lastAccessedAt,
					})
				}
			}
		})

		if len(beats) > 0 {
			self.flushAction(beats)
		}

		self.isFlushing.CompareAndSwap(true, false)

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
	self.apiSessionLastUpdate.Remove(id)
}
