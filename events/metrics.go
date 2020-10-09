package events

import (
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/openziti/foundation/events"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/pkg/errors"
	"reflect"
	"regexp"
)

func registerMetricsEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(MetricsEventHandler)
	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/fabric/events/MetricsEventHandler interface.", reflect.TypeOf(val))
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

	adapter := &metricsAdapter{
		sourceFilter: sourceFilter,
		metricFilter: metricFilter,
		handler:      handler,
	}

	events.AddMetricsEventHandler(adapter)
	return nil
}

type metricsAdapter struct {
	sourceFilter *regexp.Regexp
	metricFilter *regexp.Regexp
	handler      MetricsEventHandler
}

func (adapter *metricsAdapter) AcceptMetrics(msg *metrics_pb.MetricsMessage) {
	if adapter.sourceFilter != nil && !adapter.sourceFilter.Match([]byte(msg.SourceId)) {
		return
	}

	event := &MetricsEvent{
		Namespace:   "metrics",
		SourceId:    msg.SourceId,
		Timestamp:   msg.Timestamp,
		Tags:        msg.Tags,
		MetricGroup: map[string]string{},
	}

	for name, value := range msg.IntValues {
		adapter.filterIntMetric(name, value, event, name)
	}

	for name, value := range msg.FloatValues {
		adapter.filterFloatMetric(name, value, event, name)
	}

	for name, value := range msg.Meters {
		adapter.filterIntMetric(name+".count", value.Count, event, name)
		adapter.filterFloatMetric(name+".mean_rate", value.MeanRate, event, name)
		adapter.filterFloatMetric(name+".m1_rate", value.M1Rate, event, name)
		adapter.filterFloatMetric(name+".m5_rate", value.M5Rate, event, name)
		adapter.filterFloatMetric(name+".m15_rate", value.M15Rate, event, name)
	}

	for name, value := range msg.Histograms {
		adapter.filterIntMetric(name+".count", value.Count, event, name)
		adapter.filterIntMetric(name+".min", value.Min, event, name)
		adapter.filterIntMetric(name+".max", value.Max, event, name)
		adapter.filterFloatMetric(name+".mean", value.Mean, event, name)
		adapter.filterFloatMetric(name+".std_dev", value.StdDev, event, name)
		adapter.filterFloatMetric(name+".variance", value.Variance, event, name)
		adapter.filterFloatMetric(name+".p50", value.P50, event, name)
		adapter.filterFloatMetric(name+".p75", value.P75, event, name)
		adapter.filterFloatMetric(name+".p95", value.P95, event, name)
		adapter.filterFloatMetric(name+".p99", value.P99, event, name)
		adapter.filterFloatMetric(name+".p999", value.P999, event, name)
		adapter.filterFloatMetric(name+".p9999", value.P9999, event, name)
	}

	if len(event.IntMetrics) > 0 || len(event.FloatMetrics) > 0 {
		adapter.handler.AcceptMetricsEvent(event)
	}
}

func (adapter *metricsAdapter) filterIntMetric(name string, value int64, event *MetricsEvent, group string) {
	if adapter.nameMatches(name) {
		if event.IntMetrics == nil {
			event.IntMetrics = make(map[string]int64)
		}
		event.IntMetrics[name] = value
		event.MetricGroup[name] = group
	}
}

func (adapter *metricsAdapter) filterFloatMetric(name string, value float64, event *MetricsEvent, group string) {
	if adapter.nameMatches(name) {
		if event.FloatMetrics == nil {
			event.FloatMetrics = make(map[string]float64)
		}
		event.FloatMetrics[name] = value
		event.MetricGroup[name] = group
	}
}

func (adapter *metricsAdapter) nameMatches(name string) bool {
	return adapter.metricFilter == nil || adapter.metricFilter.Match([]byte(name))
}

type MetricsEvent struct {
	Namespace    string
	SourceId     string
	Timestamp    *timestamp.Timestamp
	Tags         map[string]string
	IntMetrics   map[string]int64
	FloatMetrics map[string]float64
	MetricGroup  map[string]string
}

type MetricsEventHandler interface {
	AcceptMetricsEvent(event *MetricsEvent)
}
