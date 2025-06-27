package main

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/common/outputz"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SimMetricsValidator struct {
	lock       sync.Mutex
	events     map[*model.Host][]*MetricsEvent
	collecting atomic.Bool
}

func (self *SimMetricsValidator) AddToModel(m *model.Model) {
	m.MetricsHandlers = append(m.MetricsHandlers, self)
}

func (self *SimMetricsValidator) StartCollecting(run model.Run) error {
	self.collecting.Store(true)
	self.lock.Lock()
	self.events = map[*model.Host][]*MetricsEvent{}
	self.lock.Unlock()
	return nil
}

func (self *SimMetricsValidator) AcceptHostMetrics(host *model.Host, event *model.MetricsEvent) {
	if !self.collecting.Load() {
		return
	}

	self.lock.Lock()
	defer self.lock.Unlock()

	simMetrics := &MetricsEvent{
		Metrics: map[string]*ServiceMetrics{},
	}

	self.events[host] = append(self.events[host], simMetrics)

	for k, v := range event.Metrics {
		set, ok := v.(model.MetricSet)
		if !ok {
			continue
		}
		if strings.HasPrefix(k, "service.") {
			parts := strings.Split(k, ":")
			metricName := strings.TrimPrefix(parts[0], "service.")
			serviceName := parts[1]
			metrics := simMetrics.getServiceMetrics(serviceName)

			switch metricName {
			case "connect.successes":
				metrics.PopulateMeter(&metrics.successes, set)
			case "connect.failures":
				metrics.PopulateMeter(&metrics.failures, set)
			case "connect.times":
				metrics.PopulateTimer(&metrics.connectTimes, set)
			case "latency":
				metrics.PopulateTimer(&metrics.latency, set)
			case "tx.bytes":
				metrics.PopulateMeter(&metrics.throughput, set)
			case "tx.messages", "rx.bytes", "rx.messages", "active", "completed":
			default:
				fmt.Printf("ignoring metric %s for service %s\n", metricName, serviceName)
			}
		}
	}

	var errList []error
	for service, metric := range simMetrics.Metrics {
		if metric.err != nil {
			errList = append(errList, metric.err)
		}
		if metric.successes == nil {
			metric.successes = &model.Meter{}
			errList = append(errList, fmt.Errorf("missing successes metric for service %s", service))
		}
		if metric.failures == nil {
			metric.failures = &model.Meter{}
			errList = append(errList, fmt.Errorf("missing failures metric for service %s", service))
		}
		if metric.connectTimes == nil {
			metric.connectTimes = &model.Timer{}
			errList = append(errList, fmt.Errorf("missing connect times metric for service %s", service))
		}
		if metric.latency == nil {
			metric.latency = &model.Timer{}
			errList = append(errList, fmt.Errorf("missing latency metric for service %s", service))
		}
		if metric.throughput == nil {
			metric.throughput = &model.Meter{}
			errList = append(errList, fmt.Errorf("missing throughput metric for service %s", service))
		}
	}
	if len(errList) > 0 {
		for name := range event.Metrics {
			fmt.Printf("metric reported: %s\n", name)
		}
		pfxlog.Logger().WithError(errors.Join(errList...)).Warn("inconsistencies detected during metrics processing")
	}
}

func (self *SimMetricsValidator) ValidateCollected() error {
	self.collecting.Store(false)

	log := pfxlog.Logger()
	log.Infof("validating metrics for %d hosts", len(self.events))

	start := time.Now()
	eventCount := 0
	for eventCount == 0 {
		self.lock.Lock()
		eventCount = len(self.events)
		self.lock.Unlock()

		if eventCount == 0 {
			if time.Since(start) > 10*time.Second {
				return errors.New("timed out waiting for any metric event to arrive")
			}
			time.Sleep(1 * time.Second)
		}
	}

	self.lock.Lock()
	defer self.lock.Unlock()

	latencyMeanThresholds := map[string]time.Duration{
		"sdk to sdk":       200 * time.Millisecond,
		"sdk to sdk-xg":    200 * time.Millisecond,
		"sdk to ert":       200 * time.Millisecond,
		"sdk-xg to sdk":    200 * time.Millisecond,
		"sdk-xg to sdk-xg": 200 * time.Millisecond,
		"sdk-xg to ert":    200 * time.Millisecond,
		"ert to sdk":       200 * time.Millisecond,
		"ert to sdk-xg":    200 * time.Millisecond,
		"ert to ert":       200 * time.Millisecond,
	}

	sdkP95Latency := 750 * time.Millisecond
	ertP95Latency := 500 * time.Millisecond
	sdkXgP5Latency := 500 * time.Millisecond

	latencyP95Thresholds := map[string]time.Duration{
		"sdk to sdk":       sdkP95Latency,
		"sdk to sdk-xg":    sdkP95Latency,
		"sdk to ert":       sdkP95Latency,
		"sdk-xg to sdk":    sdkP95Latency,
		"ert to sdk":       sdkP95Latency,
		"ert to ert":       ertP95Latency,
		"ert to sdk-xg":    ertP95Latency,
		"sdk-xg to ert":    ertP95Latency,
		"sdk-xg to sdk-xg": sdkXgP5Latency,
	}

	sdkThroughput := uint64(750) * 1024
	ertThroughput := 4 * uint64(1000) * 1024
	sdkXgThroughput := 10 * uint64(1000) * 1024

	throughputThresholds := map[string]uint64{
		"sdk to sdk":       sdkThroughput,
		"sdk to sdk-xg":    sdkThroughput,
		"sdk to ert":       sdkThroughput,
		"ert to sdk":       sdkThroughput,
		"sdk-xg to sdk":    sdkThroughput,
		"sdk-xg to ert":    ertThroughput,
		"ert to sdk-xg":    ertThroughput,
		"ert to ert":       ertThroughput,
		"sdk-xg to sdk-xg": sdkXgThroughput,
	}

	var errList []error

	for host, events := range self.events {
		for idx, event := range events {
			isLast := idx == len(events)-1
			for service, metrics := range event.Metrics {
				clientType := self.getFullType(host.Id, service)
				if metrics.err != nil {
					errList = append(errList, metrics.err)
				}

				if isLast {
					if metrics.failures.Count > 0 {
						errList = append(errList, fmt.Errorf("%s: service %s has %v failures", host.Id, service, metrics.failures.Count))
					}
					if metrics.successes.Count == 0 {
						errList = append(errList, fmt.Errorf("%s: service %s has no successes", host.Id, service))
					}
				}

				// ignore the first latency measurement
				if idx > 5 && strings.Contains(service, "latency") {
					meanLatency := time.Duration(int64(metrics.latency.Mean))
					meanLatencyThreshold := latencyMeanThresholds[clientType]
					if meanLatency > meanLatencyThreshold {
						err := fmt.Errorf("%s: service %s has mean latency %s exceeding %s",
							host.Id, service, meanLatency.String(), meanLatencyThreshold.String())
						fmt.Printf("outlier intermediary: %v\n", err)
					}

					p95Latency := time.Duration(int64(metrics.latency.P95))
					p95LatencyThreshold := latencyP95Thresholds[clientType]
					if p95Latency > p95LatencyThreshold {
						err := fmt.Errorf("%s: service %s has p95 latency %s exceeding %s",
							host.Id, service, p95Latency.String(), p95LatencyThreshold.String())
						fmt.Printf("outlier intermediary: %v\n", err)
					}
				}

				if isLast && strings.Contains(service, "latency") {
					meanLatency := time.Duration(int64(metrics.latency.Mean))
					meanLatencyThreshold := latencyMeanThresholds[clientType]
					if meanLatency > meanLatencyThreshold {
						err := fmt.Errorf("%s: service %s has mean latency %s exceeding %s",
							host.Id, service, meanLatency.String(), meanLatencyThreshold.String())
						errList = append(errList, err)
					}

					p95Latency := time.Duration(int64(metrics.latency.P95))
					p95LatencyThreshold := latencyP95Thresholds[clientType]
					if p95Latency > p95LatencyThreshold {
						err := fmt.Errorf("%s: service %s has p95 latency %s exceeding %s",
							host.Id, service, p95Latency.String(), p95LatencyThreshold.String())
						errList = append(errList, err)
					}
				}

				if isLast && strings.Contains(service, "throughput") {
					throughputThreshold := throughputThresholds[clientType]

					if uint64(metrics.throughput.M1Rate) < throughputThreshold {
						errList = append(errList, fmt.Errorf("%s: service %s has throughput %v not meeting %s",
							host.Id, service, outputz.FormatBytes(uint64(metrics.throughput.M1Rate)),
							outputz.FormatBytes(throughputThreshold)))
					}
				}

				if isLast {
					if strings.Contains(service, "latency") {
						fmt.Printf("%s: service %s has mean latency %s, p95 latency %s\n", host.Id, service,
							time.Duration(int64(metrics.latency.Mean)).String(), time.Duration(int64(metrics.latency.P95)).String())
					} else {
						fmt.Printf("%s: service %s has throughput %v\n", host.Id, service,
							outputz.FormatBytes(uint64(metrics.throughput.M1Rate)))
					}
				}
			}
		}
	}

	if len(errList) > 0 {
		self.LogMetrics()
	}

	self.events = map[*model.Host][]*MetricsEvent{}

	pfxlog.Logger().Infof("metric validation complete with %d errors", len(errList))

	return errors.Join(errList...)
}

func (self *SimMetricsValidator) getFullType(hostType, serviceName string) string {
	return fmt.Sprintf("%s to %s", self.getType(hostType), self.getType(serviceName))
}

func (self *SimMetricsValidator) getType(name string) string {
	if strings.Contains(name, "-xg") {
		return "sdk-xg"
	}
	if strings.Contains(name, "ert") {
		return "ert"
	}
	return "sdk"
}

func (self *SimMetricsValidator) LogMetrics() {
	for host, events := range self.events {
		fmt.Printf("metrics for host %s\n", host.Id)
		for idx, event := range events {
			for service, metrics := range event.Metrics {
				self.printIndentF(4, "metrics for service %s, index: %d", service, idx)
				self.LogMeter("connect.successes", metrics.successes)
				self.LogMeter("connect.failures", metrics.failures)
				self.LogTimer("connect.times", metrics.connectTimes)
				if strings.Contains(service, "latency") {
					self.LogTimer("latency", metrics.latency)
				}
				if !strings.Contains(service, "latency") {
					self.LogMeter("tx.bytes", metrics.throughput)
				}
			}
		}
	}
}

func (self *SimMetricsValidator) LogTimer(name string, meter *model.Timer) {
	duration := func(f float64) string {
		return time.Duration(int64(f)).String()
	}

	self.printIndentF(8, "timer: %s", name)
	self.printIndentF(12, "count    : %d", meter.Count)
	self.printIndentF(12, "mean_rate: %f", meter.MeanRate)
	self.printIndentF(12, "m1_rate : %f", meter.M1Rate)
	self.printIndentF(12, "mean    : %s", duration(meter.Mean))
	self.printIndentF(12, "min     : %s", time.Duration(meter.Min).String())
	self.printIndentF(12, "max     : %s", time.Duration(meter.Max).String())
	self.printIndentF(12, "p50     : %s", duration(meter.P50))
	self.printIndentF(12, "p75     : %s", duration(meter.P75))
	self.printIndentF(12, "p95     : %s", duration(meter.P95))
	self.printIndentF(12, "p99     : %s", duration(meter.P99))
}

func (self *SimMetricsValidator) LogMeter(name string, meter *model.Meter) {
	self.printIndentF(8, "meter: %s", name)
	self.printIndentF(12, "count    : %d", meter.Count)
	if !strings.HasPrefix(name, "connect.") {
		self.printIndentF(12, "m1_rate  : %f", meter.M1Rate)
		self.printIndentF(12, "m5_rate  : %f", meter.M5Rate)
		self.printIndentF(12, "mean_rate: %f", meter.MeanRate)
	}
}

func (self *SimMetricsValidator) printIndentF(indent int, format string, args ...interface{}) {
	for i := 0; i < indent; i++ {
		fmt.Printf(" ")
	}
	fmt.Printf(format, args...)
	fmt.Printf("\n")
}

type MetricsEvent struct {
	Metrics map[string]*ServiceMetrics
}

func (self *MetricsEvent) getServiceMetrics(name string) *ServiceMetrics {
	if v, ok := self.Metrics[name]; ok {
		return v
	}
	result := &ServiceMetrics{}
	self.Metrics[name] = result
	return result
}

type ServiceMetrics struct {
	successes    *model.Meter
	failures     *model.Meter
	connectTimes *model.Timer
	latency      *model.Timer
	throughput   *model.Meter
	err          error
}

func (self *ServiceMetrics) PopulateMeter(val **model.Meter, metric model.MetricSet) {
	meter, err := metric.AsMeter()
	if err != nil {
		self.err = err
		return
	}
	*val = meter
}

func (self *ServiceMetrics) PopulateTimer(val **model.Timer, metric model.MetricSet) {
	timer, err := metric.AsTimer()
	if err != nil {
		self.err = err
		return
	}
	*val = timer
}
