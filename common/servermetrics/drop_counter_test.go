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
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/openziti/metrics"
	"github.com/stretchr/testify/require"
)

type recordingHandler struct {
	sync.Mutex
	records []slog.Record
}

func (self *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (self *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	self.Lock()
	defer self.Unlock()
	self.records = append(self.records, r)
	return nil
}

func (self *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return self }
func (self *recordingHandler) WithGroup(string) slog.Handler      { return self }

func (self *recordingHandler) dropCounts() []int64 {
	self.Lock()
	defer self.Unlock()
	var result []int64
	for _, r := range self.records {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "dropped" {
				result = append(result, a.Value.Int64())
			}
			return true
		})
	}
	return result
}

func TestDropCounterLogsOneSummaryPerInterval(t *testing.T) {
	closeNotify := make(chan struct{})
	defer close(closeNotify)

	registry := metrics.NewRegistry("test", nil)
	defer registry.DisposeAll()

	handler := &recordingHandler{}
	counter := newDropCounter(registry, "test.dropped", slog.New(handler), "work dropped", time.Hour, closeNotify)

	for range 5 {
		counter.Drop()
	}
	require.Empty(t, handler.dropCounts(), "Drop must not log")

	counter.report()
	require.Equal(t, []int64{5}, handler.dropCounts())

	counter.report()
	require.Equal(t, []int64{5}, handler.dropCounts(), "an interval with no drops logs nothing")

	counter.Drop()
	counter.report()
	require.Equal(t, []int64{5, 1}, handler.dropCounts())

	require.Equal(t, int64(6), registry.Meter("test.dropped").Count())
}

func TestDropCounterReportsOnInterval(t *testing.T) {
	closeNotify := make(chan struct{})
	defer close(closeNotify)

	handler := &recordingHandler{}
	counter := newDropCounter(nil, "", slog.New(handler), "work dropped", 10*time.Millisecond, closeNotify)
	counter.Drop()
	counter.Drop()

	require.Eventually(t, func() bool {
		counts := handler.dropCounts()
		return len(counts) == 1 && counts[0] == 2
	}, 2*time.Second, 5*time.Millisecond)
}
