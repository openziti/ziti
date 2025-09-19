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

package model

import (
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	cmap "github.com/orcaman/concurrent-map/v2"
)

type HeartbeatCollector struct {
	apiSessionLastAccessedAtMap cmap.ConcurrentMap[string, *HeartbeatStatus]
	updateInterval              time.Duration
	closeNotify                 <-chan struct{}
	isFlushing                  atomic.Bool
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
		closeNotify:                 env.GetCloseNotifyChannel(),
	}

	env.GetStores().ApiSession.AddEntityIdListener(collector.Remove, boltz.EntityDeleted)

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

func (self *HeartbeatCollector) flush() {
	if self.isFlushing.CompareAndSwap(false, true) {
		defer self.isFlushing.CompareAndSwap(true, false)
		pfxlog.Logger().Trace("flushing heartbeat collector")

		var beats []*Heartbeat

		var buckets [][]*Heartbeat

		self.apiSessionLastAccessedAtMap.IterCb(func(key string, status *HeartbeatStatus) {
			if len(beats) >= self.batchSize {
				buckets = append(buckets, beats)
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

		for _, bucket := range buckets {
			self.flushAction(bucket)
		}

		if len(beats) > 0 {
			self.flushAction(beats)
		}

	} else {
		pfxlog.Logger().Warn("attempting to flush heartbeats from collector, a flush is already in progress, skipping")
	}
}

func (self *HeartbeatCollector) Remove(id string) {
	self.apiSessionLastAccessedAtMap.Remove(id)
}
