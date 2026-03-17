package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
)

type MetricsDumper struct {
	lock     sync.Mutex
	runLabel string
	file     *os.File
	eventIdx int
}

func NewMetricsDumper() *MetricsDumper {
	return &MetricsDumper{}
}

func (self *MetricsDumper) AddToModel(m *model.Model) {
	m.MetricsHandlers = append(m.MetricsHandlers, self)
}

func (self *MetricsDumper) SetRunLabel(label string) {
	self.lock.Lock()
	defer self.lock.Unlock()

	if self.file != nil {
		_ = self.file.Close()
		self.file = nil
	}

	self.runLabel = label
	self.eventIdx = 0

	safeName := strings.ReplaceAll(label, " ", "_")
	safeName = strings.ReplaceAll(safeName, "(", "")
	safeName = strings.ReplaceAll(safeName, ")", "")

	dir := "/tmp/circuit-perf-diff"
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, fmt.Sprintf("metrics-%s.txt", safeName))

	f, err := os.Create(path)
	if err != nil {
		pfxlog.Logger().WithError(err).Errorf("failed to create metrics dump file %s", path)
		return
	}
	self.file = f
	pfxlog.Logger().Infof("dumping metrics to %s", path)
}

func (self *MetricsDumper) Close() {
	self.lock.Lock()
	defer self.lock.Unlock()
	if self.file != nil {
		_ = self.file.Close()
		self.file = nil
	}
}

func (self *MetricsDumper) AcceptHostMetrics(host *model.Host, event *model.MetricsEvent) {
	self.lock.Lock()
	defer self.lock.Unlock()

	if self.file == nil {
		return
	}

	self.eventIdx++
	_, _ = fmt.Fprintf(self.file, "=== Event #%d host=%s time=%s ===\n", self.eventIdx, host.Id, event.Timestamp.Format("15:04:05.000"))

	// Sort metric names for consistent output
	names := make([]string, 0, len(event.Metrics))
	for k := range event.Metrics {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, k := range names {
		v := event.Metrics[k]
		set, ok := v.(model.MetricSet)
		if !ok {
			_, _ = fmt.Fprintf(self.file, "  %s = %v\n", k, v)
			continue
		}

		_, _ = fmt.Fprintf(self.file, "  %s:\n", k)
		self.dumpMetricSet(set, "    ")
	}
	_, _ = fmt.Fprintf(self.file, "\n")
}

func (self *MetricsDumper) dumpMetricSet(set model.MetricSet, indent string) {
	// Try known types first
	if meter, err := set.AsMeter(); err == nil {
		_, _ = fmt.Fprintf(self.file, "%scount=%d mean_rate=%.2f m1=%.2f m5=%.2f\n",
			indent, meter.Count, meter.MeanRate, meter.M1Rate, meter.M5Rate)
		return
	}

	if timer, err := set.AsTimer(); err == nil {
		_, _ = fmt.Fprintf(self.file, "%scount=%d mean=%.0fns p50=%.0fns p95=%.0fns p99=%.0fns min=%dns max=%dns\n",
			indent, timer.Count, timer.Mean, timer.P50, timer.P95, timer.P99, timer.Min, timer.Max)
		return
	}

	// Fall back to dumping all keys
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(self.file, "%s%s=%v\n", indent, k, set[k])
	}
}
