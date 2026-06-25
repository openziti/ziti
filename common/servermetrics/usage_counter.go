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
	"time"

	"github.com/openziti/metrics"
)

// UsageSource identifies the bucket and tags a usage update belongs to.
type UsageSource interface {
	GetIntervalId() string
	GetTags() map[string]string
}

// A UsageCounter allows tracking usage bucketized by some interval.
type UsageCounter interface {
	metrics.Metric
	Update(source UsageSource, usageType string, time time.Time, value uint64)
}

type usageReporter interface {
	reportUsage(intervalStartUTC int64, intervalLength time.Duration, values map[string]*usageSet)
}

func newUsageCounter(name string,
	intervalSize time.Duration,
	ageThreshold time.Duration,
	reporter usageReporter,
	disposeF func(),
	eventChan chan func()) *usageCounterImpl {

	currentInterval := time.Now().Truncate(intervalSize).UTC().Unix()
	currentCounters := map[string]*usageSet{}
	intervalMap := map[int64]map[string]*usageSet{}
	intervalMap[currentInterval] = currentCounters

	intervalCounter := &usageCounterImpl{
		name:            name,
		reporter:        reporter,
		intervalSize:    intervalSize,
		currentInterval: currentInterval,
		currentValues:   currentCounters,
		intervalMap:     intervalMap,
		eventChan:       eventChan,
		ageThreshold:    ageThreshold,
		dispose:         disposeF,
	}

	return intervalCounter
}

type usageSet struct {
	values map[string]uint64
	tags   map[string]string
}

type usageCounterImpl struct {
	name            string
	reporter        usageReporter
	intervalSize    time.Duration
	currentInterval int64
	currentValues   map[string]*usageSet
	intervalMap     map[int64]map[string]*usageSet
	eventChan       chan func()
	ageThreshold    time.Duration
	dispose         func()
}

func (self *usageCounterImpl) Update(source UsageSource, usageType string, time time.Time, value uint64) {
	if value == 0 {
		return
	}

	self.eventChan <- func() {
		interval := time.Truncate(self.intervalSize).UTC().Unix()
		valueMap := self.getValueMapForInterval(interval)
		set := valueMap[source.GetIntervalId()]
		if set == nil {
			set = &usageSet{
				values: map[string]uint64{},
				tags:   source.GetTags(),
			}
			valueMap[source.GetIntervalId()] = set
		}
		set.values[usageType] += value
	}
}

func (self *usageCounterImpl) Dispose() {
	self.dispose()
}

func (self *usageCounterImpl) getValueMapForInterval(interval int64) map[string]*usageSet {
	if self.currentInterval == interval {
		return self.currentValues
	}

	if result, found := self.intervalMap[interval]; found {
		return result
	}

	result := map[string]*usageSet{}
	self.intervalMap[interval] = result

	if interval > self.currentInterval {
		self.currentInterval = interval
		self.currentValues = result
	}

	return result
}

func (self *usageCounterImpl) flushIntervals() {
	threshold := time.Now().UTC().Add(-self.ageThreshold).Unix()

	for interval, values := range self.intervalMap {
		if interval < self.currentInterval || interval <= threshold {
			if len(values) > 0 {
				self.reporter.reportUsage(interval, self.intervalSize, values)
				if interval == self.currentInterval {
					self.currentValues = map[string]*usageSet{}
					self.intervalMap[self.currentInterval] = self.currentValues
				} else {
					delete(self.intervalMap, interval)
				}
			} else {
				delete(self.intervalMap, interval)
			}
		}
	}
}
