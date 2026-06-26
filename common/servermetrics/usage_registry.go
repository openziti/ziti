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
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/servermetrics/metrics_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
)

const (
	DefaultIntervalAgeThreshold = 80 * time.Second

	MinEventQueueSize     = 16
	DefaultEventQueueSize = 256
)

// UsageRegistry extends a metrics registry with interval and usage counters and
// produces ziti's MetricsMessage wire format. It embeds metrics.Registry (so
// it can be used anywhere a base registry is expected, including the sdk xgress
// machinery) and layers the reporting/usage subsystem ziti owns on top. The wire
// poll is exposed as PollMessage rather than Poll to avoid colliding with the
// embedded library's Poll, which returns the library's own (unused) message type.
type UsageRegistry interface {
	metrics.Registry
	PollMessage() *metrics_pb.MetricsMessage
	PollMessageWithoutUsageMetrics() *metrics_pb.MetricsMessage
	IntervalCounter(name string, intervalSize time.Duration) IntervalCounter
	UsageCounter(name string, intervalSize time.Duration) UsageCounter
	FlushToHandler(handler Handler)
	StartReporting(eventSink Handler, reportInterval time.Duration, msgQueueSize int)
}

// UsageRegistryConfig configures a UsageRegistry.
type UsageRegistryConfig struct {
	SourceId             string
	Tags                 map[string]string
	EventQueueSize       int
	CloseNotify          <-chan struct{}
	IntervalAgeThreshold time.Duration
}

// DefaultUsageRegistryConfig returns a UsageRegistryConfig with default queue and
// interval-age settings for the given source id.
func DefaultUsageRegistryConfig(sourceId string, closeNotify <-chan struct{}) UsageRegistryConfig {
	return UsageRegistryConfig{
		SourceId:             sourceId,
		Tags:                 map[string]string{},
		CloseNotify:          closeNotify,
		EventQueueSize:       DefaultEventQueueSize,
		IntervalAgeThreshold: DefaultIntervalAgeThreshold,
	}
}

// NewUsageRegistry creates a UsageRegistry backed by a base metrics.Registry.
func NewUsageRegistry(config UsageRegistryConfig) UsageRegistry {
	if config.EventQueueSize < MinEventQueueSize {
		config.EventQueueSize = MinEventQueueSize
	}

	if config.IntervalAgeThreshold <= 0 {
		config.IntervalAgeThreshold = DefaultIntervalAgeThreshold
	}

	return &usageRegistryImpl{
		Registry:             metrics.NewRegistry(config.SourceId, config.Tags),
		sourceId:             config.SourceId,
		tags:                 config.Tags,
		intervalMetrics:      cmap.New[intervalMetric](),
		eventChan:            make(chan func(), config.EventQueueSize),
		closeNotify:          config.CloseNotify,
		intervalAgeThreshold: config.IntervalAgeThreshold,
	}
}

type bucketEvent struct {
	interval *metrics_pb.MetricsMessage_IntervalCounter
	name     string
}

type intervalMetric interface {
	flushIntervals()
}

type usageRegistryImpl struct {
	metrics.Registry
	sourceId             string
	tags                 map[string]string
	intervalMetrics      cmap.ConcurrentMap[string, intervalMetric]
	eventChan            chan func()
	intervalBuckets      []*bucketEvent
	usageBuckets         []*metrics_pb.MetricsMessage_UsageCounter
	closeNotify          <-chan struct{}
	lock                 sync.Mutex
	intervalAgeThreshold time.Duration
}

func (self *usageRegistryImpl) StartReporting(eventSink Handler, reportInterval time.Duration, msgQueueSize int) {
	msgEvents := make(chan *messageBuilder, msgQueueSize)
	go self.run(reportInterval, msgEvents)
	go self.sendMsgs(eventSink, msgEvents)
}

// IntervalCounter creates or returns an IntervalCounter. Interval counters are
// tracked by the usage registry directly rather than in the base registry, since
// their buckets feed the wire message rather than the visitor-based snapshot.
func (self *usageRegistryImpl) IntervalCounter(name string, intervalSize time.Duration) IntervalCounter {
	self.lock.Lock()
	defer self.lock.Unlock()

	if metric, present := self.intervalMetrics.Get(name); present {
		intervalCounter, ok := metric.(IntervalCounter)
		if !ok {
			panic(fmt.Errorf("metric '%v' already exists and is not an interval counter. It is a %v", name, reflect.TypeOf(metric).Name()))
		}
		return intervalCounter
	}

	disposeF := func() { self.intervalMetrics.Remove(name) }
	intervalCounter := newIntervalCounter(name, intervalSize, self.intervalAgeThreshold, self.eventChan, self, disposeF)
	self.intervalMetrics.Set(name, intervalCounter)
	return intervalCounter
}

// UsageCounter creates or returns a UsageCounter.
func (self *usageRegistryImpl) UsageCounter(name string, intervalSize time.Duration) UsageCounter {
	self.lock.Lock()
	defer self.lock.Unlock()

	if metric, present := self.intervalMetrics.Get(name); present {
		counter, ok := metric.(UsageCounter)
		if !ok {
			panic(fmt.Errorf("metric '%v' already exists and is not a usage counter. It is a %v", name, reflect.TypeOf(metric).Name()))
		}
		return counter
	}

	disposeF := func() { self.intervalMetrics.Remove(name) }
	usageCounter := newUsageCounter(name, intervalSize, self.intervalAgeThreshold, self, disposeF, self.eventChan)
	self.intervalMetrics.Set(name, usageCounter)
	return usageCounter
}

// PollMessage returns a MetricsMessage including the base metrics plus any
// accumulated interval and usage buckets.
func (self *usageRegistryImpl) PollMessage() *metrics_pb.MetricsMessage {
	base := pollRegistry(self.Registry, self.sourceId, self.tags)
	if base == nil && self.intervalBuckets == nil {
		return nil
	}

	var builder *messageBuilder
	if base == nil {
		builder = newMessageBuilder(self.sourceId, self.tags)
	} else {
		builder = (*messageBuilder)(base)
	}

	builder.addIntervalBucketEvents(self.intervalBuckets)
	self.intervalBuckets = nil

	builder.UsageCounters = self.usageBuckets
	self.usageBuckets = nil

	sort.Slice(builder.UsageCounters, func(i, j int) bool {
		return builder.UsageCounters[i].IntervalStartUTC < builder.UsageCounters[j].IntervalStartUTC
	})

	return (*metrics_pb.MetricsMessage)(builder)
}

// PollMessageWithoutUsageMetrics returns a MetricsMessage of the base metrics
// only, excluding interval and usage buckets.
func (self *usageRegistryImpl) PollMessageWithoutUsageMetrics() *metrics_pb.MetricsMessage {
	return pollRegistry(self.Registry, self.sourceId, self.tags)
}

func (self *usageRegistryImpl) reportInterval(counter *intervalCounterImpl, intervalStartUTC int64, values map[string]uint64) {
	bucket := &metrics_pb.MetricsMessage_IntervalBucket{
		IntervalStartUTC: intervalStartUTC,
		Values:           values,
	}

	interval := &metrics_pb.MetricsMessage_IntervalCounter{
		IntervalLength: uint64(counter.intervalSize.Seconds()),
		Buckets:        []*metrics_pb.MetricsMessage_IntervalBucket{bucket},
	}

	self.intervalBuckets = append(self.intervalBuckets, &bucketEvent{
		interval: interval,
		name:     counter.name,
	})
}

func (self *usageRegistryImpl) reportUsage(intervalStartUTC int64, intervalLength time.Duration, values map[string]*usageSet) {
	counter := &metrics_pb.MetricsMessage_UsageCounter{
		IntervalStartUTC: intervalStartUTC,
		IntervalLength:   uint64(intervalLength.Seconds()),
		Buckets:          map[string]*metrics_pb.MetricsMessage_UsageBucket{},
	}

	for k, v := range values {
		counter.Buckets[k] = &metrics_pb.MetricsMessage_UsageBucket{
			Values: v.values,
			Tags:   v.tags,
		}
	}
	self.usageBuckets = append(self.usageBuckets, counter)
}

func (self *usageRegistryImpl) run(reportInterval time.Duration, msgEvents chan *messageBuilder) {
	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	for {
		select {
		case event := <-self.eventChan:
			event()
		case <-ticker.C:
			select {
			case msgEvents <- self.flushAndPoll():
			case <-self.closeNotify:
				return
			}
		case <-self.closeNotify:
			self.DisposeAll()
			return
		}
	}
}

func (self *usageRegistryImpl) flushAndPoll() *messageBuilder {
	for entry := range self.intervalMetrics.IterBuffered() {
		entry.Val.flushIntervals()
	}

	builder := newMessageBuilder(self.sourceId, self.tags)

	if len(self.intervalBuckets) > 0 {
		builder.addIntervalBucketEvents(self.intervalBuckets)
		self.intervalBuckets = nil
	}

	if len(self.usageBuckets) > 0 {
		builder.UsageCounters = self.usageBuckets
		self.usageBuckets = nil

		sort.Slice(builder.UsageCounters, func(i, j int) bool {
			return builder.UsageCounters[i].IntervalStartUTC < builder.UsageCounters[j].IntervalStartUTC
		})
	}

	return builder
}

func (self *usageRegistryImpl) pollAppend(builder *messageBuilder) *metrics_pb.MetricsMessage {
	self.Registry.AcceptVisitor(builder)
	if builder.isEmpty() {
		return nil
	}
	return (*metrics_pb.MetricsMessage)(builder)
}

func (self *usageRegistryImpl) sendMsgs(eventSink Handler, msgEvents chan *messageBuilder) {
	for {
		select {
		case builder := <-msgEvents:
			if msg := self.pollAppend(builder); msg != nil {
				eventSink.AcceptMetrics(msg)
			}
		case <-self.closeNotify:
			return
		}
	}
}

// FlushAndPoll flushes interval metrics and returns the resulting message,
// running the flush on the registry's event loop for consistency.
func (self *usageRegistryImpl) FlushAndPoll() *metrics_pb.MetricsMessage {
	msgC := make(chan *metrics_pb.MetricsMessage, 1)
	self.eventChan <- func() {
		builder := self.flushAndPoll()
		msgC <- self.pollAppend(builder)
	}
	return <-msgC
}

func (self *usageRegistryImpl) FlushToHandler(handler Handler) {
	if msg := self.FlushAndPoll(); msg != nil {
		handler.AcceptMetrics(msg)
	}
}
