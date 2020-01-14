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
	"github.com/golang/protobuf/ptypes"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/rcrowley/go-metrics"
	"time"
)

type messageBuilder ctrl_pb.MetricsMessage

func newMessageBuilder(sourceType ctrl_pb.MetricsSourceType, sourceId string, tags map[string]string) *messageBuilder {
	now := time.Now()
	nowTS, err := ptypes.TimestampProto(now)
	if err != nil {
		pfxlog.Logger().Errorf("The now time (%v) is out of range for valid timestamps. Your clock is wrong", now)
	}
	builder := &messageBuilder{Timestamp: nowTS}
	builder.SourceType = sourceType
	builder.SourceId = sourceId
	builder.Tags = tags

	return builder
}

func (builder *messageBuilder) addIntValue(name string, value int64) {
	if builder.IntValues == nil {
		builder.IntValues = make(map[string]int64)
	}
	builder.IntValues[name] = value
}

func (builder *messageBuilder) addCounter(name string, metric metrics.Counter) {
	builder.addIntValue(name, metric.Count())
}

func (builder *messageBuilder) addIntGauge(name string, metric metrics.Gauge) {
	builder.addIntValue(name, metric.Value())
}

func (builder *messageBuilder) addFloatGauge(name string, metric metrics.GaugeFloat64) {
	if builder.FloatValues == nil {
		builder.FloatValues = make(map[string]float64)
	}
	builder.FloatValues[name] = metric.Value()
}

func (builder *messageBuilder) addMeter(name string, metric metrics.Meter) {
	meter := &ctrl_pb.MetricsMessage_Meter{}
	meter.Count = metric.Count()
	meter.M1Rate = metric.Rate1()
	meter.M5Rate = metric.Rate5()
	meter.M15Rate = metric.Rate15()
	meter.MeanRate = metric.RateMean()

	if builder.Meters == nil {
		builder.Meters = make(map[string]*ctrl_pb.MetricsMessage_Meter)
	}

	builder.Meters[name] = meter
}

func (builder *messageBuilder) addHistogram(name string, metric metrics.Histogram) {
	histogram := &ctrl_pb.MetricsMessage_Histogram{}
	histogram.Count = metric.Count()
	histogram.Max = metric.Max()
	histogram.Mean = metric.Mean()
	histogram.Min = metric.Min()
	histogram.StdDev = metric.StdDev()
	histogram.Variance = metric.Variance()

	ps := metric.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999, 0.9999})

	histogram.P50 = ps[0]
	histogram.P75 = ps[1]
	histogram.P95 = ps[2]
	histogram.P99 = ps[3]
	histogram.P999 = ps[4]
	histogram.P9999 = ps[5]

	if builder.Histograms == nil {
		builder.Histograms = make(map[string]*ctrl_pb.MetricsMessage_Histogram)
	}

	builder.Histograms[name] = histogram
}

func (builder *messageBuilder) addIntervalBucketEvents(events []*bucketEvent) {
	for _, event := range events {
		if builder.IntervalCounters == nil {
			builder.IntervalCounters = make(map[string]*ctrl_pb.MetricsMessage_IntervalCounter)
		}
		counter, present := builder.IntervalCounters[event.name]
		if !present {
			builder.IntervalCounters[event.name] = event.interval
		} else {
			counter.Buckets = append(counter.Buckets, event.interval.Buckets...)
		}
	}
}
