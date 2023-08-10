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

package events

import (
	"github.com/google/uuid"
	"github.com/openziti/fabric/controller/event"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/metrics/metrics_pb"
	"github.com/pkg/errors"
	"reflect"
	"regexp"
)

func (self *Dispatcher) AddMetricsEventHandler(handler event.MetricsEventHandler) {
	self.metricsEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveMetricsEventHandler(handler event.MetricsEventHandler) {
	self.metricsEventHandlers.DeleteIf(func(val event.MetricsEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.MetricsEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})

	self.metricsMsgEventHandlers.DeleteIf(func(val event.MetricsMessageHandler) bool {
		if w, ok := val.(event.MetricsEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) AddMetricsMessageHandler(handler event.MetricsMessageHandler) {
	self.metricsMsgEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveMetricsMessageHandler(handler event.MetricsMessageHandler) {
	self.metricsMsgEventHandlers.Delete(handler)
}

func (self *Dispatcher) AcceptMetricsEvent(event *event.MetricsEvent) {
	go func() {
		for _, handler := range self.metricsEventHandlers.Value() {
			handler.AcceptMetricsEvent(event)
		}
	}()
}

func (self *Dispatcher) AcceptMetricsMsg(msg *metrics_pb.MetricsMessage) {
	go func() {
		for _, handler := range self.metricsMsgEventHandlers.Value() {
			handler.AcceptMetricsMsg(msg)
		}
	}()
}

func (self *Dispatcher) initMetricsEvents(n *network.Network) {
	self.AddMetricsMessageHandler(n)
	self.AddMetricsMessageHandler(event.MetricsMessageHandlerF(self.relayMessagesToEventsUnfiltered))
}

func (self *Dispatcher) relayMessagesToEventsUnfiltered(msg *metrics_pb.MetricsMessage) {
	if len(self.metricsEventHandlers.Value()) > 0 {
		self.convertMetricsMsgToEvents(msg, nil, nil, self)
	}
}

func (self *Dispatcher) registerMetricsEventHandler(val interface{}, config map[string]interface{}) error {
	handler, ok := val.(event.MetricsEventHandler)
	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/event/MetricsEventHandler interface.", reflect.TypeOf(val))
	}

	var sourceFilterDef = ""
	if sourceRegexVal, ok := config["sourceFilter"]; ok {
		sourceFilterDef, ok = sourceRegexVal.(string)
		if !ok {
			return errors.Errorf("invalid sourceFilter value %v of type %v. must be string", sourceRegexVal, reflect.TypeOf(sourceRegexVal))
		}
	}

	var sourceFilter *regexp.Regexp
	var err error
	if sourceFilterDef != "" {
		if sourceFilter, err = regexp.Compile(sourceFilterDef); err != nil {
			return err
		}
	}

	var metricFilterDef = ""
	if metricRegexVal, ok := config["metricFilter"]; ok {
		metricFilterDef, ok = metricRegexVal.(string)
		if !ok {
			return errors.Errorf("invalid metricFilter value %v of type %v. must be string", metricRegexVal, reflect.TypeOf(metricRegexVal))
		}
	}

	var metricFilter *regexp.Regexp
	if metricFilterDef != "" {
		if metricFilter, err = regexp.Compile(metricFilterDef); err != nil {
			return err
		}
	}

	adapter := self.NewFilteredMetricsAdapter(sourceFilter, metricFilter, handler)
	self.AddMetricsMessageHandler(adapter)
	return nil
}

func (self *Dispatcher) unregisterMetricsEventHandler(val interface{}) {
	if handler, ok := val.(event.MetricsEventHandler); ok {
		self.RemoveMetricsEventHandler(handler)
	}
}

func (self *Dispatcher) newMetricEvent(msg *metrics_pb.MetricsMessage, metricType string, name string, id string) *event.MetricsEvent {
	result := &event.MetricsEvent{
		MetricType:    metricType,
		Namespace:     event.MetricsEventsNs,
		SourceAppId:   msg.SourceId,
		Timestamp:     msg.Timestamp.AsTime(),
		Metric:        name,
		Tags:          msg.Tags,
		SourceEventId: id,
		Version:       event.MetricsEventsVersion,
	}

	for _, mapper := range self.metricsMappers.Value() {
		mapper(msg, result)
	}

	return result
}

func (self *Dispatcher) convertMetricsMsgToEvents(msg *metrics_pb.MetricsMessage,
	sourceFilter *regexp.Regexp,
	metricFilter *regexp.Regexp,
	handler event.MetricsEventHandler) {

	if sourceFilter != nil && !sourceFilter.Match([]byte(msg.SourceId)) {
		return
	}

	parentEventId := uuid.NewString()

	for name, value := range msg.IntValues {
		evt := self.newMetricEvent(msg, "intValue", name, parentEventId)
		self.filterMetric(metricFilter, "", value, evt)
		self.finishEvent(evt, handler)
	}

	for name, value := range msg.FloatValues {
		evt := self.newMetricEvent(msg, "floatValue", name, parentEventId)
		self.filterMetric(metricFilter, "", value, evt)
		self.finishEvent(evt, handler)
	}

	for name, value := range msg.Meters {
		evt := self.newMetricEvent(msg, "meter", name, parentEventId)
		self.filterMetric(metricFilter, "count", value.Count, evt)
		self.filterMetric(metricFilter, "mean_rate", value.MeanRate, evt)
		self.filterMetric(metricFilter, "m1_rate", value.M1Rate, evt)
		self.filterMetric(metricFilter, "m5_rate", value.M5Rate, evt)
		self.filterMetric(metricFilter, "m15_rate", value.M15Rate, evt)
		self.finishEvent(evt, handler)
	}

	for name, value := range msg.Histograms {
		evt := self.newMetricEvent(msg, "histogram", name, parentEventId)
		self.filterMetric(metricFilter, "count", value.Count, evt)
		self.filterMetric(metricFilter, "min", value.Min, evt)
		self.filterMetric(metricFilter, "max", value.Max, evt)
		self.filterMetric(metricFilter, "mean", value.Mean, evt)
		self.filterMetric(metricFilter, "std_dev", value.StdDev, evt)
		self.filterMetric(metricFilter, "variance", value.Variance, evt)
		self.filterMetric(metricFilter, "p50", value.P50, evt)
		self.filterMetric(metricFilter, "p75", value.P75, evt)
		self.filterMetric(metricFilter, "p95", value.P95, evt)
		self.filterMetric(metricFilter, "p99", value.P99, evt)
		self.filterMetric(metricFilter, "p999", value.P999, evt)
		self.filterMetric(metricFilter, "p9999", value.P9999, evt)
		self.finishEvent(evt, handler)
	}

	for name, value := range msg.Timers {
		evt := self.newMetricEvent(msg, "timer", name, parentEventId)
		self.filterMetric(metricFilter, "count", value.Count, evt)

		self.filterMetric(metricFilter, "mean_rate", value.MeanRate, evt)
		self.filterMetric(metricFilter, "m1_rate", value.M1Rate, evt)
		self.filterMetric(metricFilter, "m5_rate", value.M5Rate, evt)
		self.filterMetric(metricFilter, "m15_rate", value.M15Rate, evt)

		self.filterMetric(metricFilter, "min", value.Min, evt)
		self.filterMetric(metricFilter, "max", value.Max, evt)
		self.filterMetric(metricFilter, "mean", value.Mean, evt)
		self.filterMetric(metricFilter, "std_dev", value.StdDev, evt)
		self.filterMetric(metricFilter, "variance", value.Variance, evt)
		self.filterMetric(metricFilter, "p50", value.P50, evt)
		self.filterMetric(metricFilter, "p75", value.P75, evt)
		self.filterMetric(metricFilter, "p95", value.P95, evt)
		self.filterMetric(metricFilter, "p99", value.P99, evt)
		self.filterMetric(metricFilter, "p999", value.P999, evt)
		self.filterMetric(metricFilter, "p9999", value.P9999, evt)
		self.finishEvent(evt, handler)
	}

}

func (self *Dispatcher) filterMetric(metricFilter *regexp.Regexp, key string, value interface{}, event *event.MetricsEvent) {
	name := event.Metric + "." + key
	if self.metricNameMatches(metricFilter, name) {
		if event.Metrics == nil {
			event.Metrics = make(map[string]interface{})
		}
		if key == "" {
			event.Metrics["value"] = value
		} else {
			event.Metrics[key] = value
		}
	}
}

func (self *Dispatcher) finishEvent(event *event.MetricsEvent, handler event.MetricsEventHandler) {
	if len(event.Metrics) > 0 {
		handler.AcceptMetricsEvent(event)
	}
}

func (self *Dispatcher) metricNameMatches(metricFilter *regexp.Regexp, name string) bool {
	return metricFilter == nil || metricFilter.Match([]byte(name))
}

func (self *Dispatcher) NewFilteredMetricsAdapter(sourceFilter *regexp.Regexp, metricFilter *regexp.Regexp, handler event.MetricsEventHandler) event.MetricsMessageHandler {
	adapter := &filteringMetricsMessageAdapter{
		dispatcher:   self,
		sourceFilter: sourceFilter,
		metricFilter: metricFilter,
		handler:      handler,
	}

	return adapter
}

type filteringMetricsMessageAdapter struct {
	dispatcher   *Dispatcher
	sourceFilter *regexp.Regexp
	metricFilter *regexp.Regexp
	handler      event.MetricsEventHandler
}

func (self *filteringMetricsMessageAdapter) IsWrapping(value event.MetricsEventHandler) bool {
	if self.handler == value {
		return true
	}
	if w, ok := self.handler.(event.MetricsEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}

func (self *filteringMetricsMessageAdapter) AcceptMetricsMsg(msg *metrics_pb.MetricsMessage) {
	if msg.DoNotPropagate {
		return
	}
	self.dispatcher.convertMetricsMsgToEvents(msg, self.sourceFilter, self.metricFilter, self.handler)
}
