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
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/ziti/zititest/zitirest"
)

const TrafficCollectorName = "traffic-results-collector"

// TrafficEvent mirrors the oidc-test-client's trafficEvent structure. ClientId
// identifies the dial target (the prox-c component in proxy mode, or the
// sdk-direct client itself) so the collector can track per-target coverage.
// Port is the local TCP port dialed in proxy mode; 0 for sdk-direct.
type TrafficEvent struct {
	Type      string    `json:"type"`
	Service   string    `json:"service"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	LatencyMs int64     `json:"latency_ms,omitempty"`
	ClientId  string    `json:"client_id"`
	Port      int       `json:"port,omitempty"`
	Timestamp time.Time `json:"ts"`
}

// TrafficResultsCollector listens on a Ziti service for traffic events from
// oidc-test-client instances. Events are stored in memory for validation queries.
type TrafficResultsCollector struct {
	mu        sync.RWMutex
	events    []TrafficEvent
	clearTime time.Time // events with timestamps before this are dropped on ingestion
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
		if self.clearTime.IsZero() || !evt.Timestamp.Before(self.clearTime) {
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
	self.clearTime = time.Now().UTC()
	self.mu.Unlock()
}

// ErrorsSince returns all error events with timestamps after the given time.
func (self *TrafficResultsCollector) ErrorsSince(since time.Time) []TrafficEvent {
	self.mu.RLock()
	defer self.mu.RUnlock()

	var result []TrafficEvent
	for _, evt := range self.events {
		if evt.Status == "error" && !evt.Timestamp.Before(since) {
			result = append(result, evt)
		}
	}
	return result
}

// SuccessCount returns the number of successful events since the given time.
func (self *TrafficResultsCollector) SuccessCount(since time.Time) int {
	self.mu.RLock()
	defer self.mu.RUnlock()

	count := 0
	for _, evt := range self.events {
		if evt.Status == "ok" && !evt.Timestamp.Before(since) {
			count++
		}
	}
	return count
}

// ErrorCount returns the number of error events since the given time.
func (self *TrafficResultsCollector) ErrorCount(since time.Time) int {
	self.mu.RLock()
	defer self.mu.RUnlock()

	count := 0
	for _, evt := range self.events {
		if evt.Status == "error" && !evt.Timestamp.Before(since) {
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

// ClientIdsWithSuccessSince returns the set of target ClientIds that have
// recorded at least one successful event since the given time. The ClientId
// identifies the dial target (the prox-c component being dialed in proxy mode,
// or the sdk-direct client itself), so this set directly measures per-target
// coverage for convergence checks.
func (self *TrafficResultsCollector) ClientIdsWithSuccessSince(since time.Time) map[string]bool {
	return self.clientIdsWithStatusSince(since, "ok")
}

// ClientIdsWithErrorSince returns the set of target ClientIds that have
// recorded at least one error event since the given time.
func (self *TrafficResultsCollector) ClientIdsWithErrorSince(since time.Time) map[string]bool {
	return self.clientIdsWithStatusSince(since, "error")
}

func (self *TrafficResultsCollector) clientIdsWithStatusSince(since time.Time, status string) map[string]bool {
	self.mu.RLock()
	defer self.mu.RUnlock()

	result := make(map[string]bool)
	for _, evt := range self.events {
		if evt.Status != status || evt.Timestamp.Before(since) {
			continue
		}
		result[evt.ClientId] = true
	}
	return result
}

// ClientHealth captures the most recent success/error timestamps seen for a
// target since a cutoff. A target whose LatestSuccess is non-zero and at-or-
// after LatestError is considered currently healthy: a recent error followed
// by a recent success (the common post-warmup pattern) registers as healthy,
// while a target whose latest event is an error is not.
type ClientHealth struct {
	LatestSuccess time.Time
	LatestError   time.Time
}

// Healthy reports whether this target's most recent event was a success.
// A target with no success event (LatestSuccess zero) is not healthy.
func (h ClientHealth) Healthy() bool {
	return !h.LatestSuccess.IsZero() && !h.LatestSuccess.Before(h.LatestError)
}

// ClientHealthSince returns the health state (latest success/error timestamps)
// for every target that has emitted at least one event since the given time.
// Callers can compare against an expected set to determine coverage and apply
// Healthy() to each entry to determine current state.
func (self *TrafficResultsCollector) ClientHealthSince(since time.Time) map[string]ClientHealth {
	self.mu.RLock()
	defer self.mu.RUnlock()

	result := make(map[string]ClientHealth)
	for _, evt := range self.events {
		if evt.Timestamp.Before(since) {
			continue
		}
		h := result[evt.ClientId]
		switch evt.Status {
		case "ok":
			if evt.Timestamp.After(h.LatestSuccess) {
				h.LatestSuccess = evt.Timestamp
			}
		case "error":
			if evt.Timestamp.After(h.LatestError) {
				h.LatestError = evt.Timestamp
			}
		}
		result[evt.ClientId] = h
	}
	return result
}
