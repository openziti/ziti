package main

import (
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/common/outputz"
	"strings"
	"sync"
	"time"
)

type SimMetricsValidator struct {
	lock   sync.Mutex
	events map[*model.Host][]*MetricsEvent
}

func (self *SimMetricsValidator) AddToModel(m *model.Model) {
	m.MetricsHandlers = append(m.MetricsHandlers, self)
}

func (self *SimMetricsValidator) AcceptHostMetrics(host *model.Host, event *model.MetricsEvent) {
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
	self.lock.Lock()
	defer self.lock.Unlock()

	for host, events := range self.events {
		for idx, event := range events {
			isLast := idx == len(events)-1
			for service, metrics := range event.Metrics {
				if metrics.err != nil {
					return metrics.err
				}
				if metrics.failures.Count > 0 {
					return fmt.Errorf("%s: service %s has %v failures", host.Id, service, metrics.failures.Count)
				}
				if metrics.successes.Count == 0 && isLast {
					return fmt.Errorf("%s: service %s has no successes", host.Id, service)
				}

				if strings.Contains(service, "latency") {
					meanLatency := time.Duration(int64(metrics.latency.Mean))
					if meanLatency > 150*time.Millisecond {
						return fmt.Errorf("%s: service %s has mean latency %s", host.Id, service, meanLatency.String())
					}

					p95Latency := time.Duration(int64(metrics.latency.P95))
					if p95Latency > 300*time.Millisecond {
						return fmt.Errorf("%s: service %s has p95 latency %s", host.Id, service, p95Latency.String())
					}
				}

				if isLast && strings.Contains(service, "throughput") {
					if metrics.throughput.M1Rate < 50*1000*1024 {
						return fmt.Errorf("%s: service %s has throughput %v", host.Id, service, outputz.FormatBytes(uint64(metrics.throughput.M1Rate)))
					}
				}

				if isLast {
					fmt.Printf("%s: service %s has mean latency %s, p95 latency %s, throughput %v\n", host.Id, service,
						time.Duration(int64(metrics.latency.Mean)).String(), time.Duration(int64(metrics.latency.P95)).String(),
						outputz.FormatBytes(uint64(metrics.throughput.M1Rate)))
				}
			}
		}
	}

	self.events = map[*model.Host][]*MetricsEvent{}

	return nil
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
