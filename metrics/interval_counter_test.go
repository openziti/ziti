/*
	Copyright 2019 NetFoundry, Inc.

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

package metrics

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sort"
	"testing"
	"time"
)

type collectingReporter struct {
	intervalChan chan *intervalEvent
}

type intervalEvent struct {
	intervalStartUTC int64
	intervalEndUTC   int64
	values           map[string]uint64
}

type intervalSlice []*intervalEvent

func (s intervalSlice) Len() int {
	return len(([]*intervalEvent)(s))
}

func (s intervalSlice) Less(i, j int) bool {
	slice := ([]*intervalEvent)(s)
	return slice[i].intervalStartUTC < slice[j].intervalStartUTC
}

func (s intervalSlice) Swap(i, j int) {
	slice := ([]*intervalEvent)(s)
	tmp := slice[i]
	slice[i] = slice[j]
	slice[j] = tmp
}

func (reporter *collectingReporter) reportInterval(counter *intervalCounterImpl, intervalStartUTC int64, values map[string]uint64) {
	i := &intervalEvent{
		intervalStartUTC: intervalStartUTC,
		intervalEndUTC:   intervalStartUTC + int64(counter.intervalSize.Seconds()),
		values:           values,
	}

	reporter.intervalChan <- i
}

func (reporter *collectingReporter) GetNextIntervals(eventCount uint32, timeout time.Duration) []*intervalEvent {
	count := uint32(0)
	var intervals []*intervalEvent
	for count < eventCount {
		select {
		case next := <-reporter.intervalChan:
			count++
			intervals = append(intervals, next)
		case <-time.After(timeout):
			return intervals
		}
	}
	return intervals
}

func TestTrackerNonDuplicate(t *testing.T) {
	assert := require.New(t)

	reporter := &collectingReporter{intervalChan: make(chan *intervalEvent)}

	intervalCounter := newIntervalCounter(
		"usage", time.Minute, reporter, time.Duration(0), time.Duration(0), func() {}).(*intervalCounterImpl)

	currentMinute := time.Now().Truncate(time.Minute)

	intervalID1 := uuid.New().String()
	intervalCounter.Update(intervalID1, currentMinute.Add(time.Second*5), 11111)
	intervalCounter.report()

	intervals := reporter.GetNextIntervals(1, time.Second)

	assert.Equal(1, len(intervals))
	interval := intervals[0]
	assert.Equal(currentMinute.UTC().Unix(), interval.intervalStartUTC)
	assert.Equal(currentMinute.Add(time.Minute).UTC().Unix(), interval.intervalEndUTC)
	assert.Equal(1, len(interval.values))
	assert.Equal(uint64(11111), interval.values[intervalID1])

	// verify that we don't see any new events
	intervalCounter.report()

	intervals = reporter.GetNextIntervals(1, time.Millisecond*50)
	assert.Equal(0, len(intervals))

	intervalID2 := uuid.New().String()

	// interval N
	intervalCounter.Update(intervalID1, currentMinute.Add(time.Second*5), 10)
	intervalCounter.Update(intervalID2, currentMinute.Add(time.Second*6), 11)

	intervalCounter.Update(intervalID1, currentMinute.Add(time.Second*25), 15)
	intervalCounter.Update(intervalID2, currentMinute.Add(time.Second*26), 17)

	intervalCounter.Update(intervalID1, currentMinute.Add(time.Second*31), 40)
	intervalCounter.Update(intervalID2, currentMinute.Add(time.Second*31), 50)

	// interval N+1
	intervalCounter.Update(intervalID1, currentMinute.Add(time.Second*62), 22)
	intervalCounter.Update(intervalID2, currentMinute.Add(time.Second*62), 23)

	// interval N-1
	intervalCounter.Update(intervalID1, currentMinute.Add(-time.Second*30), 123)
	intervalCounter.Update(intervalID1, currentMinute.Add(-1), 77)
	intervalCounter.Update(intervalID2, currentMinute.Add(-time.Second*30), 321)

	// interval N-2
	intervalCounter.Update(intervalID1, currentMinute.Add(-time.Second*70), 234)

	// interval N-3
	intervalCounter.Update(intervalID2, currentMinute.Add(-time.Second*121), 567)

	intervalCounter.report()

	intervals = reporter.GetNextIntervals(5, time.Millisecond*50)
	fmt.Printf("Current minute: %v\n", currentMinute.UTC().Unix())
	for _, interval = range intervals {
		fmt.Printf("Interval: %v\n", interval.intervalStartUTC)
	}
	assert.Equal(4, len(intervals)) // Future minutes won't be reported
	sort.Sort(intervalSlice(intervals))

	// interval N-3
	interval = intervals[0]
	assert.Equal(currentMinute.Add(time.Minute * - 3).UTC().Unix(), interval.intervalStartUTC)
	assert.Equal(currentMinute.Add(time.Minute * - 2).UTC().Unix(), interval.intervalEndUTC)
	assert.Equal(1, len(interval.values))
	assert.Equal(uint64(567), interval.values[intervalID2])

	// interval N-2
	interval = intervals[1]
	assert.Equal(currentMinute.Add(time.Minute * - 2).UTC().Unix(), interval.intervalStartUTC)
	assert.Equal(currentMinute.Add(time.Minute * - 1).UTC().Unix(), interval.intervalEndUTC)
	assert.Equal(1, len(interval.values))
	assert.Equal(uint64(234), interval.values[intervalID1])

	// interval N-1
	interval = intervals[2]
	assert.Equal(currentMinute.Add(time.Minute * - 1).UTC().Unix(), interval.intervalStartUTC)
	assert.Equal(currentMinute.UTC().Unix(), interval.intervalEndUTC)
	assert.Equal(2, len(interval.values))
	assert.Equal(uint64(200), interval.values[intervalID1])
	assert.Equal(uint64(321), interval.values[intervalID2])

	// interval N
	interval = intervals[3]
	assert.Equal(currentMinute.UTC().Unix(), interval.intervalStartUTC)
	assert.Equal(currentMinute.Add(time.Minute).UTC().Unix(), interval.intervalEndUTC)
	assert.Equal(2, len(interval.values))
	assert.Equal(uint64(65), interval.values[intervalID1])
	assert.Equal(uint64(78), interval.values[intervalID2])
}
