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

package zitilib_runlevel_5_operation

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/zitirest"
)

const TrafficCollectorName = "traffic-results-collector"

// TrafficEvent mirrors the oidc-test-client's trafficEvent structure.
type TrafficEvent struct {
	Type       string `json:"type"`
	Service    string `json:"service"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	LatencyMs  int64  `json:"latency_ms,omitempty"`
	ProxyPort  int    `json:"proxy_port,omitempty"`
	ProxyIndex int    `json:"proxy_index,omitempty"`
	ClientId   string `json:"client_id"`
	Timestamp  string `json:"ts"`
}

// TrafficResultsCollector listens on a Ziti service for traffic events from
// oidc-test-client instances. Events are stored in memory for validation queries.
type TrafficResultsCollector struct {
	mu        sync.RWMutex
	events    []TrafficEvent
	clearTime string // RFC3339; events with timestamps before this are dropped on ingestion
	started   atomic.Bool
}

// SetupCollectorIdentity creates and enrolls the collector identity using the
// provided management API clients.
func (self *TrafficResultsCollector) SetupCollectorIdentity(run model.Run, clients *zitirest.Clients) error {
	return setupCollectorIdentity(run, clients, TrafficCollectorName, "traffic-collector")
}

// StartCollecting begins listening on the given Ziti service.
func (self *TrafficResultsCollector) StartCollecting(run model.Run, service string) error {
	if !self.started.CompareAndSwap(false, true) {
		return nil
	}

	configPath := run.GetLabel().GetFilePath(TrafficCollectorName + ".json")
	cfg, err := ziti.NewConfigFromFile(configPath)
	if err != nil {
		return err
	}

	ctx, err := ziti.NewContext(cfg)
	if err != nil {
		return err
	}

	listener, err := ctx.Listen(service)
	if err != nil {
		return err
	}

	go func() {
		log := pfxlog.Logger()
		log.Infof("traffic results collector listening on service %q", service)
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.WithError(err).Info("traffic results listener closed")
				return
			}
			go self.handleConnection(conn)
		}
	}()

	return nil
}

// StartCollectingStage returns a model.Stage that starts traffic result collection.
func (self *TrafficResultsCollector) StartCollectingStage(service string) model.Stage {
	return model.StageActionF(func(run model.Run) error {
		return self.StartCollecting(run, service)
	})
}

func (self *TrafficResultsCollector) handleConnection(conn net.Conn) {
	defer conn.Close()
	log := pfxlog.Logger()
	log.Infof("new traffic results connection from %s", conn.RemoteAddr())

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		var evt TrafficEvent
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
			continue
		}

		self.mu.Lock()
		if self.clearTime == "" || evt.Timestamp >= self.clearTime {
			self.events = append(self.events, evt)
		}
		self.mu.Unlock()
	}
}

// Snapshot returns a copy of all events collected so far and clears the buffer.
func (self *TrafficResultsCollector) Snapshot() []TrafficEvent {
	self.mu.Lock()
	defer self.mu.Unlock()
	result := self.events
	self.events = nil
	return result
}

// Clear discards all collected events and sets a watermark so that stale
// events arriving on old connections are dropped.
func (self *TrafficResultsCollector) Clear() {
	self.mu.Lock()
	self.events = nil
	self.clearTime = time.Now().UTC().Format(time.RFC3339)
	self.mu.Unlock()
}

// ErrorsSince returns all error events with timestamps after the given time.
func (self *TrafficResultsCollector) ErrorsSince(since time.Time) []TrafficEvent {
	self.mu.RLock()
	defer self.mu.RUnlock()

	sinceStr := since.UTC().Format(time.RFC3339)
	var result []TrafficEvent
	for _, evt := range self.events {
		if evt.Status == "error" && evt.Timestamp >= sinceStr {
			result = append(result, evt)
		}
	}
	return result
}

// SuccessCount returns the number of successful events since the given time.
func (self *TrafficResultsCollector) SuccessCount(since time.Time) int {
	self.mu.RLock()
	defer self.mu.RUnlock()

	sinceStr := since.UTC().Format(time.RFC3339)
	count := 0
	for _, evt := range self.events {
		if evt.Status == "ok" && evt.Timestamp >= sinceStr {
			count++
		}
	}
	return count
}

// ErrorCount returns the number of error events since the given time.
func (self *TrafficResultsCollector) ErrorCount(since time.Time) int {
	self.mu.RLock()
	defer self.mu.RUnlock()

	sinceStr := since.UTC().Format(time.RFC3339)
	count := 0
	for _, evt := range self.events {
		if evt.Status == "error" && evt.Timestamp >= sinceStr {
			count++
		}
	}
	return count
}

// TotalCount returns the total number of events collected.
func (self *TrafficResultsCollector) TotalCount() int {
	self.mu.RLock()
	defer self.mu.RUnlock()
	return len(self.events)
}
