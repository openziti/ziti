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

	"github.com/google/uuid"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/servermetrics/metrics_pb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler is a sink for metric messages. It is ziti's own handler type, decoupled
// from the metrics library so ziti owns the wire format (metrics_pb.MetricsMessage).
type Handler interface {
	// AcceptMetrics is called when new metrics become available.
	AcceptMetrics(message *metrics_pb.MetricsMessage)
}

// Poll builds a MetricsMessage snapshot of the given registry. It walks the
// registry through the library's public visitor, so it works on any
// metrics.Registry without reaching into the library's internals. Returns nil
// when the registry has no reportable metrics. Tags are not carried because ziti
// creates its registries without tags.
func Poll(registry metrics.Registry) *metrics_pb.MetricsMessage {
	return pollRegistry(registry, registry.SourceId(), nil)
}

func pollRegistry(registry metrics.Registry, sourceId string, tags map[string]string) *metrics_pb.MetricsMessage {
	builder := newMessageBuilder(sourceId, tags)
	registry.AcceptVisitor(builder)
	if builder.isEmpty() {
		return nil
	}
	return (*metrics_pb.MetricsMessage)(builder)
}

// messageBuilder accumulates metrics into a MetricsMessage. It implements
// metrics.Visitor so it can be populated from any registry via AcceptVisitor,
// replacing the library's internal type-switch.
type messageBuilder metrics_pb.MetricsMessage

func newMessageBuilder(sourceId string, tags map[string]string) *messageBuilder {
	return &messageBuilder{
		EventId:   uuid.NewString(),
		Timestamp: timestamppb.New(time.Now()),
		SourceId:  sourceId,
		Tags:      tags,
	}
}

// isEmpty reports whether the builder accumulated any reportable metric, matching
// the library's "nothing to report -> nil" behavior.
func (builder *messageBuilder) isEmpty() bool {
	return len(builder.IntValues) == 0 &&
		len(builder.FloatValues) == 0 &&
		len(builder.Meters) == 0 &&
		len(builder.Histograms) == 0 &&
		len(builder.Timers) == 0 &&
		len(builder.IntervalCounters) == 0 &&
		len(builder.UsageCounters) == 0
}

func (builder *messageBuilder) addIntValue(name string, value int64) {
	if builder.IntValues == nil {
		builder.IntValues = make(map[string]int64)
	}
	builder.IntValues[name] = value
}

func (builder *messageBuilder) VisitGauge(name string, metric metrics.Gauge) {
	builder.addIntValue(name, metric.Value())
}

func (builder *messageBuilder) VisitGaugeFloat64(name string, metric metrics.GaugeFloat64) {
	if builder.FloatValues == nil {
		builder.FloatValues = make(map[string]float64)
	}
	builder.FloatValues[name] = metric.Value()
}

func (builder *messageBuilder) VisitMeter(name string, metric metrics.Meter) {
	meter := &metrics_pb.MetricsMessage_Meter{
		Count:    metric.Count(),
		M1Rate:   metric.Rate1(),
		M5Rate:   metric.Rate5(),
		M15Rate:  metric.Rate15(),
		MeanRate: metric.RateMean(),
	}
	if builder.Meters == nil {
		builder.Meters = make(map[string]*metrics_pb.MetricsMessage_Meter)
	}
	builder.Meters[name] = meter
}

func (builder *messageBuilder) VisitHistogram(name string, metric metrics.Histogram) {
	histogram := &metrics_pb.MetricsMessage_Histogram{
		Count:    metric.Count(),
		Max:      metric.Max(),
		Mean:     metric.Mean(),
		Min:      metric.Min(),
		StdDev:   metric.StdDev(),
		Variance: metric.Variance(),
	}
	ps := metric.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999, 0.9999})
	histogram.P50 = ps[0]
	histogram.P75 = ps[1]
	histogram.P95 = ps[2]
	histogram.P99 = ps[3]
	histogram.P999 = ps[4]
	histogram.P9999 = ps[5]

	if builder.Histograms == nil {
		builder.Histograms = make(map[string]*metrics_pb.MetricsMessage_Histogram)
	}
	builder.Histograms[name] = histogram
}

func (builder *messageBuilder) VisitTimer(name string, metric metrics.Timer) {
	timer := &metrics_pb.MetricsMessage_Timer{
		Count:    metric.Count(),
		Max:      metric.Max(),
		Mean:     metric.Mean(),
		Min:      metric.Min(),
		StdDev:   metric.StdDev(),
		Variance: metric.Variance(),
	}
	ps := metric.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999, 0.9999})
	timer.P50 = ps[0]
	timer.P75 = ps[1]
	timer.P95 = ps[2]
	timer.P99 = ps[3]
	timer.P999 = ps[4]
	timer.P9999 = ps[5]

	timer.M1Rate = metric.Rate1()
	timer.M5Rate = metric.Rate5()
	timer.M15Rate = metric.Rate15()
	timer.MeanRate = metric.RateMean()

	if builder.Timers == nil {
		builder.Timers = make(map[string]*metrics_pb.MetricsMessage_Timer)
	}
	builder.Timers[name] = timer
}

func (builder *messageBuilder) addIntervalBucketEvents(events []*bucketEvent) {
	for _, event := range events {
		if builder.IntervalCounters == nil {
			builder.IntervalCounters = make(map[string]*metrics_pb.MetricsMessage_IntervalCounter)
		}
		counter, present := builder.IntervalCounters[event.name]
		if !present {
			builder.IntervalCounters[event.name] = event.interval
		} else {
			counter.Buckets = append(counter.Buckets, event.interval.Buckets...)
		}
	}
}
