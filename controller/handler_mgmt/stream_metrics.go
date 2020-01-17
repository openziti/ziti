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

package handler_mgmt

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/netfoundry/ziti-fabric/metrics"
	"github.com/netfoundry/ziti-fabric/pb/ctrl_pb"
	"github.com/netfoundry/ziti-fabric/pb/mgmt_pb"
	"github.com/netfoundry/ziti-foundation/channel2"
	"regexp"
)

type streamMetricsHandler struct {
	network        *network.Network
	streamHandlers []metrics.Handler
}

func newStreamMetricsHandler(network *network.Network) *streamMetricsHandler {
	return &streamMetricsHandler{network: network}
}

func (*streamMetricsHandler) ContentType() int32 {
	return int32(mgmt_pb.ContentType_StreamMetricsRequestType)
}

func (handler *streamMetricsHandler) HandleReceive(msg *channel2.Message, ch channel2.Channel) {
	request := &mgmt_pb.StreamMetricsRequest{}
	if err := proto.Unmarshal(msg.Body, request); err != nil {
		sendFailure(msg, ch, err.Error())
		return
	}

	filters, err := parseFilters(request)
	if err != nil {
		sendFailure(msg, ch, err.Error())
		return
	}

	metricsStreamHandler := &MetricsStreamHandler{
		ch:              ch,
		filters:         filters,
		eventController: handler.network.GetMetricsEventController(),
	}

	handler.streamHandlers = append(handler.streamHandlers, metricsStreamHandler)
	handler.network.GetMetricsEventController().AddHandler(metricsStreamHandler)
}

func (handler *streamMetricsHandler) HandleClose(ch channel2.Channel) {
	for _, streamHandler := range handler.streamHandlers {
		handler.network.GetMetricsEventController().RemoveHandler(streamHandler)
	}
}

func parseFilters(msg *mgmt_pb.StreamMetricsRequest) ([]*metricsFilter, error) {
	var filters []*metricsFilter
	for _, filterDef := range msg.Matchers {
		filter := &metricsFilter{}

		if filterDef.NameRegex != "" {
			regex, err := regexp.Compile(filterDef.NameRegex)
			if err != nil {
				return nil, err
			}
			filter.metricFilter = regex
		}

		if filterDef.NameRegex != "" {
			regex, err := regexp.Compile(filterDef.SourceIDRegex)
			if err != nil {
				return nil, err
			}
			filter.sourceFilter = regex
		}

		filters = append(filters, filter)
	}
	return filters, nil
}

type metricsFilter struct {
	sourceFilter *regexp.Regexp
	metricFilter *regexp.Regexp
}

type MetricsStreamHandler struct {
	ch              channel2.Channel
	filters         []*metricsFilter
	eventController metrics.EventController
}

func (handler *MetricsStreamHandler) AcceptMetrics(msg *ctrl_pb.MetricsMessage) {
	if handler.filters == nil {
		handler.filter(nil, msg)
		return
	}

	var filters []*metricsFilter
	for _, filter := range handler.filters {
		if filter.sourceFilter == nil || filter.sourceFilter.Match([]byte(msg.SourceId)) {
			filters = append(filters, filter)
		}
	}
	if len(filters) > 0 {
		handler.filter(filters, msg)
	}
}

func (handler *MetricsStreamHandler) filter(filters []*metricsFilter, msg *ctrl_pb.MetricsMessage) {
	event := &mgmt_pb.StreamMetricsEvent{
		SourceId:  msg.SourceId,
		Timestamp: msg.Timestamp,
		Tags:      msg.Tags,
	}

	for name, value := range msg.IntValues {
		filterIntMetric(name, value, event, filters)
	}

	for name, value := range msg.FloatValues {
		filterFloatMetric(name, value, event, filters)
	}

	for name, value := range msg.Meters {
		filterIntMetric(name+".count", value.Count, event, filters)
		filterFloatMetric(name+".mean_rate", value.MeanRate, event, filters)
		filterFloatMetric(name+".m1_rate", value.M1Rate, event, filters)
		filterFloatMetric(name+".m5_rate", value.M5Rate, event, filters)
		filterFloatMetric(name+".m15_rate", value.M15Rate, event, filters)
	}

	for name, value := range msg.Histograms {
		filterIntMetric(name+".count", value.Count, event, filters)
		filterIntMetric(name+".min", value.Min, event, filters)
		filterIntMetric(name+".max", value.Max, event, filters)
		filterFloatMetric(name+".mean", value.Mean, event, filters)
		filterFloatMetric(name+".std_dev", value.StdDev, event, filters)
		filterFloatMetric(name+".variance", value.Variance, event, filters)
		filterFloatMetric(name+".p50", value.P50, event, filters)
		filterFloatMetric(name+".p75", value.P75, event, filters)
		filterFloatMetric(name+".p95", value.P95, event, filters)
		filterFloatMetric(name+".p99", value.P99, event, filters)
		filterFloatMetric(name+".p999", value.P999, event, filters)
		filterFloatMetric(name+".p9999", value.P9999, event, filters)
	}

	for name, interval := range msg.IntervalCounters {
		if nameMatches(name, filters) {
			for _, bucket := range interval.Buckets {
				intervalMetric := &mgmt_pb.StreamMetricsEvent_IntervalMetric{
					Name: name,
				}
				intervalMetric.Values = bucket.Values
				intervalMetric.IntervalStartUTC = &timestamp.Timestamp{Seconds: bucket.IntervalStartUTC}
				intervalEndSeconds := bucket.IntervalStartUTC + int64(interval.IntervalLength)
				intervalMetric.IntervalEndUTC = &timestamp.Timestamp{Seconds: intervalEndSeconds}

				event.IntervalMetrics = append(event.IntervalMetrics, intervalMetric)
			}
		}
	}

	if len(event.IntMetrics) > 0 || len(event.FloatMetrics) > 0 || len(event.IntervalMetrics) > 0 {
		handler.send(event)
	}
}

func filterIntMetric(name string, value int64, event *mgmt_pb.StreamMetricsEvent, filters []*metricsFilter) {
	if nameMatches(name, filters) {
		if event.IntMetrics == nil {
			event.IntMetrics = make(map[string]int64)
		}
		event.IntMetrics[name] = value
	}
}

func filterFloatMetric(name string, value float64, event *mgmt_pb.StreamMetricsEvent, filters []*metricsFilter) {
	if nameMatches(name, filters) {
		if event.FloatMetrics == nil {
			event.FloatMetrics = make(map[string]float64)
		}
		event.FloatMetrics[name] = value
	}
}

func nameMatches(name string, filters []*metricsFilter) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if filter.metricFilter == nil || filter.metricFilter.Match([]byte(name)) {
			return true
		}
	}
	return false
}

func (handler *MetricsStreamHandler) send(msg *mgmt_pb.StreamMetricsEvent) {
	body, err := proto.Marshal(msg)
	if err != nil {
		pfxlog.Logger().Errorf("unexpected error serializing StreamMetricsEvent (%s)", err)
		return
	}

	responseMsg := channel2.NewMessage(int32(mgmt_pb.ContentType_StreamMetricsEventType), body)
	if err := handler.ch.Send(responseMsg); err != nil {
		pfxlog.Logger().Errorf("unexpected error sending StreamMetricsEvent (%s)", err)
		handler.close()
	}
}

func (handler *MetricsStreamHandler) close() {
	if err := handler.ch.Close(); err != nil {
		pfxlog.Logger().WithError(err).Errorf("failure while closing handler")
	}
	handler.eventController.RemoveHandler(handler)
}
