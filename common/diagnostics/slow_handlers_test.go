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

package diagnostics

import (
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/stretchr/testify/require"
)

// TestStackDumpWatch_SkipsADumpAfterTheHandlerReturned: a watchdog callback can start after the handler has
// returned and the timer was stopped. Dumping then records something else and spends the dump budget.
func TestStackDumpWatch_SkipsADumpAfterTheHandlerReturned(t *testing.T) {
	dumps := 0
	watch := &stackDumpWatch{dump: func() { dumps++ }}

	watch.disarm()
	watch.fire()

	require.Zero(t, dumps)
}

// TestStackDumpWatch_DumpsWhileTheHandlerRuns: the case the watchdog exists for, and a dump already under way
// is not undone by the handler returning during it.
func TestStackDumpWatch_DumpsWhileTheHandlerRuns(t *testing.T) {
	dumps := 0
	watch := &stackDumpWatch{dump: func() { dumps++ }}

	watch.fire()
	watch.disarm()
	watch.fire()

	require.Equal(t, 1, dumps)
}

// TestRecordHandlerDuration_SummarizesPerContentType: slow handlers are counted rather than logged one at a
// time, and each content type's summary names its slowest.
func TestRecordHandlerDuration_SummarizesPerContentType(t *testing.T) {
	req := require.New(t)

	cfg := DefaultSlowHandlerConfig()
	cfg.Enabled = true
	cfg.WarnInterval = time.Hour
	detector := NewSlowHandlerDetector("ctrl", cfg)

	detector.recordHandlerDuration(1, &idChannel{id: "a"}, 150*time.Millisecond)
	detector.recordHandlerDuration(1, &idChannel{id: "b"}, 300*time.Millisecond)
	detector.recordHandlerDuration(1, &idChannel{id: "c"}, 200*time.Millisecond)
	detector.recordHandlerDuration(2, &idChannel{id: "d"}, 50*time.Millisecond)
	detector.recordHandlerDuration(3, &idChannel{id: "e"}, 120*time.Millisecond)

	req.Len(detector.slow, 2, "a handler under the threshold is not counted")
	req.Equal(int64(3), detector.slow[1].count)
	req.Equal(300*time.Millisecond, detector.slow[1].max)
	req.Equal("b", detector.slow[1].channelId)
	req.Equal(int64(1), detector.slow[3].count)

	detector.logSlowHandlers()
	req.Nil(detector.slow, "a summary starts the count again")
}

// TestRecordHandlerDuration_SchedulesTheSummary: nothing else logs what was counted, so the first slow handler
// after a summary has to schedule the next.
func TestRecordHandlerDuration_SchedulesTheSummary(t *testing.T) {
	cfg := DefaultSlowHandlerConfig()
	cfg.Enabled = true
	cfg.WarnInterval = 10 * time.Millisecond
	detector := NewSlowHandlerDetector("ctrl", cfg)

	pending := func() bool {
		detector.slowLock.Lock()
		defer detector.slowLock.Unlock()
		return detector.slow != nil
	}

	for range 2 {
		detector.recordHandlerDuration(1, &idChannel{id: "a"}, time.Second)
		require.True(t, pending())
		require.Eventually(t, func() bool { return !pending() }, 2*time.Second, 5*time.Millisecond)
	}
}

// idChannel stands in for a channel that only has to name itself. It embeds the interface, so anything else
// panics rather than silently doing nothing.
type idChannel struct {
	channel.Channel
	id string
}

func (self *idChannel) Id() string { return self.id }
