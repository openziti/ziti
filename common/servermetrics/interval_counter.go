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

// IntervalCounter allows tracking counters which are bucketized by some interval.
type IntervalCounter interface {
	metrics.Metric
	Update(intervalId string, time time.Time, value uint64)
}

type intervalCounterReporter interface {
	reportInterval(counter *intervalCounterImpl, intervalStartUTC int64, values map[string]uint64)
}

func newIntervalCounter(name string,
	intervalSize time.Duration,
	ageThreshold time.Duration,
	eventChan chan func(),
	reporter intervalCounterReporter,
	disposeF func()) *intervalCounterImpl {

	currentInterval := time.Now().Truncate(intervalSize).UTC().Unix()
	currentCounters := make(map[string]uint64)
	intervalMap := make(map[int64]map[string]uint64)
	intervalMap[currentInterval] = currentCounters

	intervalCounter := &intervalCounterImpl{
		name:            name,
		intervalSize:    intervalSize,
		currentInterval: currentInterval,
		currentValues:   currentCounters,
		intervalMap:     intervalMap,
		ageThreshold:    ageThreshold,
		eventChan:       eventChan,
		reporter:        reporter,
		dispose:         disposeF,
	}

	return intervalCounter
}

type intervalCounterImpl struct {
	name            string
	intervalSize    time.Duration
	currentInterval int64
	currentValues   map[string]uint64
	intervalMap     map[int64]map[string]uint64
	eventChan       chan func()
	reporter        intervalCounterReporter
	ageThreshold    time.Duration
	dispose         func()
}

func (intervalCounter *intervalCounterImpl) Update(intervalId string, time time.Time, value uint64) {
	if value == 0 {
		return
	}

	intervalCounter.eventChan <- func() {
		interval := time.Truncate(intervalCounter.intervalSize).UTC().Unix()
		valueMap := intervalCounter.getValueMapForInterval(interval)
		valueMap[intervalId] += value
	}
}

func (intervalCounter *intervalCounterImpl) Dispose() {
	intervalCounter.dispose()
}

func (intervalCounter *intervalCounterImpl) getValueMapForInterval(interval int64) map[string]uint64 {
	if intervalCounter.currentInterval == interval {
		return intervalCounter.currentValues
	}

	if result, found := intervalCounter.intervalMap[interval]; found {
		return result
	}

	result := make(map[string]uint64)
	intervalCounter.intervalMap[interval] = result

	if interval > intervalCounter.currentInterval {
		intervalCounter.currentInterval = interval
		intervalCounter.currentValues = result
	}

	return result
}

func (intervalCounter *intervalCounterImpl) flushIntervals() {
	threshold := time.Now().UTC().Add(-intervalCounter.ageThreshold).Unix()

	for interval, values := range intervalCounter.intervalMap {
		if interval < intervalCounter.currentInterval || interval <= threshold {
			if len(values) > 0 {
				intervalCounter.reporter.reportInterval(intervalCounter, interval, values)
				if interval == intervalCounter.currentInterval {
					intervalCounter.currentValues = make(map[string]uint64)
					intervalCounter.intervalMap[intervalCounter.currentInterval] = intervalCounter.currentValues
				} else {
					delete(intervalCounter.intervalMap, interval)
				}
			}
		}
	}
}
