package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/sirupsen/logrus"
)

// metricSample holds the baseline and candidate values for a single metric in a single run pair.
type metricSample struct {
	baseline  float64
	candidate float64
	delta     float64 // percentage
}

type PerfDiffReport struct {
	lock       sync.Mutex
	runLabel   string
	collecting atomic.Bool

	// runLabel -> "host:service" -> list of metric snapshots
	runs map[string]map[string][]*serviceSnapshot
	// count of events per run+host (for warm-up skip)
	eventCounts map[string]int

	// Accumulated stats across run pairs.
	// Key order matches metricKeys for deterministic output.
	metricKeys []string
	allSamples map[string][]metricSample // metric key -> samples across run pairs

	refFile *os.File
}

type serviceSnapshot struct {
	throughputByterate float64
	throughputPeakM1   float64
	latencyMean        float64
	latencyP50         float64
	latencyP95         float64
	latencyP99         float64
	successes          int64
	failures           int64
}

var (
	clients  = []string{"loop-client-xg", "ert"}
	services = []string{"throughput-xg", "latency-xg", "throughput-ert", "latency-ert"}
)

func NewPerfDiffReport() *PerfDiffReport {
	return &PerfDiffReport{
		runs:        map[string]map[string][]*serviceSnapshot{},
		eventCounts: map[string]int{},
		allSamples:  map[string][]metricSample{},
	}
}

func (self *PerfDiffReport) AddToModel(m *model.Model) {
	m.MetricsHandlers = append(m.MetricsHandlers, self)
}

func (self *PerfDiffReport) OpenRefFile() {
	dir := "/tmp/circuit-perf-diff"
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "run-details.txt")
	f, err := os.Create(path)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to create reference file")
		return
	}
	self.refFile = f
	pfxlog.Logger().Infof("writing individual run details to %s", path)
}

func (self *PerfDiffReport) CloseRefFile() {
	if self.refFile != nil {
		_ = self.refFile.Close()
		self.refFile = nil
	}
}

func (self *PerfDiffReport) SetRunLabel(label string) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.runLabel = label
	self.eventCounts[label] = 0
}

func (self *PerfDiffReport) StartCollecting(_ model.Run) error {
	self.collecting.Store(true)
	return nil
}

func (self *PerfDiffReport) StopCollecting() {
	self.collecting.Store(false)
}

// ResetRuns clears collected metrics for the next run pair.
func (self *PerfDiffReport) ResetRuns() {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.runs = map[string]map[string][]*serviceSnapshot{}
	self.eventCounts = map[string]int{}
}

func (self *PerfDiffReport) AcceptHostMetrics(host *model.Host, event *model.MetricsEvent) {
	if !self.collecting.Load() {
		return
	}

	self.lock.Lock()
	defer self.lock.Unlock()

	label := self.runLabel
	if label == "" {
		return
	}

	hostLabel := label + ":" + host.Id
	self.eventCounts[hostLabel]++

	// Skip first 5 metric events per host per run (warm-up)
	if self.eventCounts[hostLabel] <= 5 {
		return
	}

	if self.runs[label] == nil {
		self.runs[label] = map[string][]*serviceSnapshot{}
	}

	serviceMap := self.runs[label]

	for k, v := range event.Metrics {
		set, ok := v.(model.MetricSet)
		if !ok {
			continue
		}
		if !strings.HasPrefix(k, "service.") {
			continue
		}

		if !strings.Contains(k, ":") {
			logrus.Warn("unexpected metric type: ", k)
			continue
		}

		parts := strings.Split(k, ":")
		metricName := strings.TrimPrefix(parts[0], "service.")
		serviceName := parts[1]

		// Key by host:service to avoid mixing metrics from different clients
		snapshotKey := host.Id + ":" + serviceName
		snap := self.getOrCreateSnapshot(serviceMap, snapshotKey)

		switch metricName {
		case "tx.byterate":
			if val, ok := set.GetFloat64Metric("value"); ok && val > 0 {
				snap.throughputByterate = val
			}
		case "tx.bytes":
			if meter, err := set.AsMeter(); err == nil {
				if meter.M1Rate > snap.throughputPeakM1 {
					snap.throughputPeakM1 = meter.M1Rate
				}
			}
		case "latency":
			if timer, err := set.AsTimer(); err == nil {
				snap.latencyMean = timer.Mean
				snap.latencyP50 = timer.P50
				snap.latencyP95 = timer.P95
				snap.latencyP99 = timer.P99
			}
		case "connect.successes":
			if meter, err := set.AsMeter(); err == nil {
				snap.successes = meter.Count
			}
		case "connect.failures":
			if meter, err := set.AsMeter(); err == nil {
				snap.failures = meter.Count
			}
		}
	}
}

func (self *PerfDiffReport) getOrCreateSnapshot(serviceMap map[string][]*serviceSnapshot, service string) *serviceSnapshot {
	snaps := serviceMap[service]
	if len(snaps) == 0 || snaps[len(snaps)-1] == nil {
		snap := &serviceSnapshot{}
		serviceMap[service] = append(snaps, snap)
		return snap
	}
	return snaps[len(snaps)-1]
}

func (self *PerfDiffReport) lastSnapshot(snaps []*serviceSnapshot) *serviceSnapshot {
	if len(snaps) == 0 {
		return &serviceSnapshot{}
	}
	return snaps[len(snaps)-1]
}

// RecordComparison extracts comparison data from the current run pair, writes it to the
// reference file, and accumulates stats. Call this after each baseline+candidate pair.
func (self *PerfDiffReport) RecordComparison(pairIdx int, baselineLabel, candidateLabel string) {
	self.lock.Lock()
	defer self.lock.Unlock()

	log := pfxlog.Logger()

	baselineRun := self.runs[baselineLabel]
	candidateRun := self.runs[candidateLabel]

	if baselineRun == nil {
		log.Errorf("pair %d: no baseline metrics collected", pairIdx)
		return
	}
	if candidateRun == nil {
		log.Errorf("pair %d: no candidate metrics collected", pairIdx)
		return
	}

	// Write individual run to reference file
	if self.refFile != nil {
		fmt.Fprintf(self.refFile, "=== Run Pair #%d ===\n\n", pairIdx)
	}

	for _, client := range clients {
		if self.refFile != nil {
			fmt.Fprintf(self.refFile, "Client: %s\n", client)
		}

		for _, svc := range services {
			key := client + ":" + svc
			baseLast := self.lastSnapshot(baselineRun[key])
			candLast := self.lastSnapshot(candidateRun[key])

			if strings.Contains(svc, "throughput") {
				self.recordThroughput(client, svc, baseLast, candLast)
			} else {
				self.recordLatency(client, svc, baseLast, candLast)
			}
		}
		if self.refFile != nil {
			fmt.Fprintf(self.refFile, "\n")
		}
	}
	if self.refFile != nil {
		fmt.Fprintf(self.refFile, "\n")
	}
}

func (self *PerfDiffReport) addSample(key string, baseline, candidate float64) {
	if _, exists := self.allSamples[key]; !exists {
		self.metricKeys = append(self.metricKeys, key)
	}
	self.allSamples[key] = append(self.allSamples[key], metricSample{
		baseline:  baseline,
		candidate: candidate,
		delta:     pctDelta(baseline, candidate),
	})
}

func (self *PerfDiffReport) recordThroughput(client, svc string, baseline, candidate *serviceSnapshot) {
	prefix := client + ":" + svc

	baseBR := baseline.throughputByterate / (1024 * 1024)
	candBR := candidate.throughputByterate / (1024 * 1024)
	self.addSample(prefix+":Throughput", baseBR, candBR)

	basePeak := baseline.throughputPeakM1 / (1024 * 1024)
	candPeak := candidate.throughputPeakM1 / (1024 * 1024)
	self.addSample(prefix+":Peak M1", basePeak, candPeak)

	if self.refFile != nil {
		deltaBR := pctDelta(baseline.throughputByterate, candidate.throughputByterate)
		deltaPeak := pctDelta(baseline.throughputPeakM1, candidate.throughputPeakM1)
		fmt.Fprintf(self.refFile, "  %s:\n", svc)
		fmt.Fprintf(self.refFile, "    Throughput:       baseline: %8.1f MB/s    candidate: %8.1f MB/s    delta: %+.1f%%\n", baseBR, candBR, deltaBR)
		fmt.Fprintf(self.refFile, "    Peak M1 Rate:     baseline: %8.1f MB/s    candidate: %8.1f MB/s    delta: %+.1f%%\n", basePeak, candPeak, deltaPeak)
		fmt.Fprintf(self.refFile, "    Successes:        baseline: %8d         candidate: %8d\n", baseline.successes, candidate.successes)
		fmt.Fprintf(self.refFile, "    Failures:         baseline: %8d         candidate: %8d\n", baseline.failures, candidate.failures)
	}
}

func (self *PerfDiffReport) recordLatency(client, svc string, baseline, candidate *serviceSnapshot) {
	prefix := client + ":" + svc

	self.addSample(prefix+":Mean", baseline.latencyMean, candidate.latencyMean)
	self.addSample(prefix+":P50", baseline.latencyP50, candidate.latencyP50)
	self.addSample(prefix+":P95", baseline.latencyP95, candidate.latencyP95)
	self.addSample(prefix+":P99", baseline.latencyP99, candidate.latencyP99)

	if self.refFile != nil {
		fmt.Fprintf(self.refFile, "  %s:\n", svc)
		writeLatencyLine(self.refFile, "Latency Mean", baseline.latencyMean, candidate.latencyMean)
		writeLatencyLine(self.refFile, "Latency P50 ", baseline.latencyP50, candidate.latencyP50)
		writeLatencyLine(self.refFile, "Latency P95 ", baseline.latencyP95, candidate.latencyP95)
		writeLatencyLine(self.refFile, "Latency P99 ", baseline.latencyP99, candidate.latencyP99)
		fmt.Fprintf(self.refFile, "    Successes:      baseline: %8d         candidate: %8d\n", baseline.successes, candidate.successes)
		fmt.Fprintf(self.refFile, "    Failures:       baseline: %8d         candidate: %8d\n", baseline.failures, candidate.failures)
	}
}

// EmitSummary prints the statistical summary across all run pairs.
func (self *PerfDiffReport) EmitSummary(baselineLabel, candidateLabel string, elapsed time.Duration) {
	self.lock.Lock()
	defer self.lock.Unlock()

	if len(self.metricKeys) == 0 {
		fmt.Println("No data collected")
		return
	}

	// Determine run pair count from first metric
	pairCount := len(self.allSamples[self.metricKeys[0]])

	fmt.Println()
	fmt.Printf("=== Statistical Summary: %s vs %s (%d run pairs over %s) ===\n",
		baselineLabel, candidateLabel, pairCount, elapsed.Truncate(time.Second))
	fmt.Println()

	currentClient := ""
	currentSvc := ""

	for _, key := range self.metricKeys {
		samples := self.allSamples[key]
		// key format: "client:service:metric"
		parts := strings.SplitN(key, ":", 3)
		if len(parts) != 3 {
			continue
		}
		client, svc, metric := parts[0], parts[1], parts[2]

		if client != currentClient {
			if currentClient != "" {
				fmt.Println()
			}
			fmt.Printf("Client: %s\n", client)
			currentClient = client
			currentSvc = ""
		}
		if svc != currentSvc {
			fmt.Printf("  %s:\n", svc)
			currentSvc = svc
		}

		isLatency := strings.Contains(svc, "latency")
		isThroughput := metric == "Throughput" || metric == "Peak M1"

		avgBase, avgCand, avgDelta, minDelta, maxDelta, stddev, better, _ := computeStats(samples, isLatency)

		if isLatency {
			fmt.Printf("    %-14s avg baseline: %12s  avg candidate: %12s\n",
				metric+":",
				time.Duration(int64(avgBase)).String(),
				time.Duration(int64(avgCand)).String())
		} else if isThroughput {
			fmt.Printf("    %-14s avg baseline: %8.1f MB/s  avg candidate: %8.1f MB/s\n",
				metric+":", avgBase, avgCand)
		}

		betterPct := float64(better) / float64(len(samples)) * 100
		fmt.Printf("      avg delta: %+6.1f%%  min: %+6.1f%%  max: %+6.1f%%  stddev: %5.1f%%  better: %d/%d (%.0f%%)\n",
			avgDelta, minDelta, maxDelta, stddev, better, len(samples), betterPct)
	}
	fmt.Println()
}

func computeStats(samples []metricSample, lowerIsBetter bool) (avgBase, avgCand, avgDelta, minDelta, maxDelta, stddev float64, better, worse int) {
	n := float64(len(samples))
	if n == 0 {
		return
	}

	minDelta = math.MaxFloat64
	maxDelta = -math.MaxFloat64

	for _, s := range samples {
		avgBase += s.baseline
		avgCand += s.candidate
		avgDelta += s.delta
		if s.delta < minDelta {
			minDelta = s.delta
		}
		if s.delta > maxDelta {
			maxDelta = s.delta
		}
		if lowerIsBetter {
			if s.delta < 0 {
				better++
			} else if s.delta > 0 {
				worse++
			}
		} else {
			if s.delta > 0 {
				better++
			} else if s.delta < 0 {
				worse++
			}
		}
	}

	avgBase /= n
	avgCand /= n
	avgDelta /= n

	// Compute stddev of deltas
	var variance float64
	for _, s := range samples {
		diff := s.delta - avgDelta
		variance += diff * diff
	}
	if n > 1 {
		stddev = math.Sqrt(variance / (n - 1))
	}

	return
}

func writeLatencyLine(f *os.File, label string, baseNs, candNs float64) {
	baseDur := time.Duration(int64(baseNs))
	candDur := time.Duration(int64(candNs))
	delta := pctDelta(baseNs, candNs)
	fmt.Fprintf(f, "    %s:  baseline: %12s    candidate: %12s    delta: %+.1f%%\n",
		label, baseDur.String(), candDur.String(), delta)
}

func pctDelta(base, candidate float64) float64 {
	if base == 0 {
		return 0
	}
	return ((candidate - base) / base) * 100
}
