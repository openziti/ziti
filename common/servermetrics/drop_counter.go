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

package servermetrics

import (
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/openziti/metrics"
)

// DropReportInterval is how often a DropCounter logs the drops it has counted.
const DropReportInterval = time.Minute

// DropCounter records work dropped under overload, such as a submission a full pool refused. Each drop
// marks a meter, and the drops are logged as one warning per interval carrying the count, never one per
// drop: the overload that causes drops fills the log queue too, and a warning waits for room in it.
type DropCounter struct {
	meter    metrics.Meter
	log      *slog.Logger
	msg      string
	interval time.Duration
	pending  atomic.Int64
}

// NewDropCounter returns a DropCounter that marks the meter named metricName in registry and logs msg at
// warn level, with the count, once per DropReportInterval in which anything was dropped. A nil registry
// records no metric. The summary is logged from a goroutine that runs until closeNotify closes; drops
// counted after the last summary are not logged.
func NewDropCounter(registry metrics.Registry, metricName string, log *slog.Logger, msg string, closeNotify <-chan struct{}) *DropCounter {
	return newDropCounter(registry, metricName, log, msg, DropReportInterval, closeNotify)
}

func newDropCounter(registry metrics.Registry, metricName string, log *slog.Logger, msg string, interval time.Duration, closeNotify <-chan struct{}) *DropCounter {
	result := &DropCounter{
		log:      log,
		msg:      msg,
		interval: interval,
	}
	if registry != nil {
		result.meter = registry.Meter(metricName)
	}
	go result.run(closeNotify)
	return result
}

// Drop records one dropped item. It never logs or blocks, so it is safe to call while holding a lock.
func (self *DropCounter) Drop() {
	if self.meter != nil {
		self.meter.Mark(1)
	}
	self.pending.Add(1)
}

func (self *DropCounter) run(closeNotify <-chan struct{}) {
	ticker := time.NewTicker(self.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			self.report()
		case <-closeNotify:
			return
		}
	}
}

func (self *DropCounter) report() {
	if dropped := self.pending.Swap(0); dropped > 0 {
		self.log.Warn(self.msg, "dropped", dropped, "interval", self.interval.String())
	}
}
