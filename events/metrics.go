package events

import (
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/uuid"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/fabric/metrics"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/pkg/errors"
	"reflect"
	"regexp"
	"strings"
)

func init() {
	AddMetricsMapper(mapLinkMetrics)
	AddMetricsMapper(mapCtrlIds)
}

func mapCtrlIds(_ *metrics_pb.MetricsMessage, event *MetricsEvent) {
	if strings.HasPrefix(event.Metric, "ctrl.") {
		parts := strings.Split(event.Metric, ":")
		event.Metric = parts[0]
		event.SourceEntityId = parts[1]
	}
}

func mapLinkMetrics(_ *metrics_pb.MetricsMessage, event *MetricsEvent) {
	if currentNetwork == nil {
		return
	}

	if strings.HasPrefix(event.Metric, "link.") {
		var name, linkId string
		if strings.HasSuffix(event.Metric, "latency") || strings.HasSuffix(event.Metric, "queue_time") {
			name, linkId = ExtractId(event.Metric, "link.", 1)
		} else {
			name, linkId = ExtractId(event.Metric, "link.", 2)
		}
		event.Metric = name
		event.SourceEntityId = linkId

		if link, _ := currentNetwork.GetLink(linkId); link != nil {
			sourceTags := event.Tags
			event.Tags = map[string]string{}
			for k, v := range sourceTags {
				event.Tags[k] = v
			}
			event.Tags["sourceRouterId"] = link.Src.Id
			event.Tags["targetRouterId"] = link.Dst.Id
		}
	}
}

type MetricsMapper func(msg *metrics_pb.MetricsMessage, event *MetricsEvent)

var metricsMappers []MetricsMapper
var currentNetwork *network.Network

func InitNetwork(n *network.Network) {
	currentNetwork = n
}

func AddMetricsMapper(mapper MetricsMapper) {
	metricsMappers = append(metricsMappers, mapper)
}

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

	metrics.AddMetricsEventHandler(adapter)
	return nil
}

func AddFilteredMetricsEventHandler(sourceFilter *regexp.Regexp, metricFilter *regexp.Regexp, handler MetricsEventHandler) func() {
	adapter := &metricsAdapter{
		sourceFilter: sourceFilter,
		metricFilter: metricFilter,
		handler:      handler,
	}

	metrics.AddMetricsEventHandler(adapter)
	return func() {
		metrics.RemoveMetricsEventHandler(adapter)
	}
}

type metricsAdapter struct {
	sourceFilter *regexp.Regexp
	metricFilter *regexp.Regexp
	handler      MetricsEventHandler
}

func (adapter *metricsAdapter) newMetricEvent(msg *metrics_pb.MetricsMessage, name string, id string) *MetricsEvent {
	result := &MetricsEvent{
		Namespace:     "metrics",
		SourceAppId:   msg.SourceId,
		Timestamp:     msg.Timestamp,
		Metric:        name,
		Tags:          msg.Tags,
		SourceEventId: id,
		Version:       2,
	}

	for _, mapper := range metricsMappers {
		mapper(msg, result)
	}

	return result
}

func (adapter *metricsAdapter) finishEvent(event *MetricsEvent) {
	if len(event.Metrics) > 0 {
		adapter.handler.AcceptMetricsEvent(event)
	}
}

func (adapter *metricsAdapter) AcceptMetrics(msg *metrics_pb.MetricsMessage) {
	if adapter.sourceFilter != nil && !adapter.sourceFilter.Match([]byte(msg.SourceId)) {
		return
	}

	parentEventId := uuid.NewString()

	for name, value := range msg.IntValues {
		event := adapter.newMetricEvent(msg, name, parentEventId)
		adapter.filterMetric("", value, event)
		adapter.finishEvent(event)
	}

	for name, value := range msg.FloatValues {
		event := adapter.newMetricEvent(msg, name, parentEventId)
		adapter.filterMetric("", value, event)
		adapter.finishEvent(event)
	}

	for name, value := range msg.Meters {
		event := adapter.newMetricEvent(msg, name, parentEventId)
		adapter.filterMetric("count", value.Count, event)
		adapter.filterMetric("mean_rate", value.MeanRate, event)
		adapter.filterMetric("m1_rate", value.M1Rate, event)
		adapter.filterMetric("m5_rate", value.M5Rate, event)
		adapter.filterMetric("m15_rate", value.M15Rate, event)
		adapter.finishEvent(event)
	}

	for name, value := range msg.Histograms {
		event := adapter.newMetricEvent(msg, name, parentEventId)
		adapter.filterMetric("count", value.Count, event)
		adapter.filterMetric("min", value.Min, event)
		adapter.filterMetric("max", value.Max, event)
		adapter.filterMetric("mean", value.Mean, event)
		adapter.filterMetric("std_dev", value.StdDev, event)
		adapter.filterMetric("variance", value.Variance, event)
		adapter.filterMetric("p50", value.P50, event)
		adapter.filterMetric("p75", value.P75, event)
		adapter.filterMetric("p95", value.P95, event)
		adapter.filterMetric("p99", value.P99, event)
		adapter.filterMetric("p999", value.P999, event)
		adapter.filterMetric("p9999", value.P9999, event)
		adapter.finishEvent(event)
	}

	for name, value := range msg.Timers {
		event := adapter.newMetricEvent(msg, name, parentEventId)
		adapter.filterMetric("count", value.Count, event)

		adapter.filterMetric("mean_rate", value.MeanRate, event)
		adapter.filterMetric("m1_rate", value.M1Rate, event)
		adapter.filterMetric("m5_rate", value.M5Rate, event)
		adapter.filterMetric("m15_rate", value.M15Rate, event)

		adapter.filterMetric("min", value.Min, event)
		adapter.filterMetric("max", value.Max, event)
		adapter.filterMetric("mean", value.Mean, event)
		adapter.filterMetric("std_dev", value.StdDev, event)
		adapter.filterMetric("variance", value.Variance, event)
		adapter.filterMetric("p50", value.P50, event)
		adapter.filterMetric("p75", value.P75, event)
		adapter.filterMetric("p95", value.P95, event)
		adapter.filterMetric("p99", value.P99, event)
		adapter.filterMetric("p999", value.P999, event)
		adapter.filterMetric("p9999", value.P9999, event)
		adapter.finishEvent(event)
	}
}

func (adapter *metricsAdapter) filterMetric(key string, value interface{}, event *MetricsEvent) {
	name := event.Metric + "." + key
	if adapter.nameMatches(name) {
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

func (adapter *metricsAdapter) nameMatches(name string) bool {
	return adapter.metricFilter == nil || adapter.metricFilter.Match([]byte(name))
}

type MetricsEvent struct {
	Namespace      string
	SourceAppId    string
	SourceEntityId string
	Version        uint32
	Timestamp      *timestamp.Timestamp
	Metric         string
	Metrics        map[string]interface{}
	Tags           map[string]string
	SourceEventId  string
}

type MetricsEventHandler interface {
	AcceptMetricsEvent(event *MetricsEvent)
}

type MetricsHandlerF func(event *MetricsEvent)

func (self MetricsHandlerF) AcceptMetricsEvent(event *MetricsEvent) {
	self(event)
}

func ExtractId(name string, prefix string, suffixLen int) (string, string) {
	rest := strings.TrimPrefix(name, prefix)
	vals := strings.Split(rest, ".")
	idVals := vals[:len(vals)-suffixLen]
	entityId := strings.Join(idVals, ".")
	return prefix + rest[len(entityId)+1:], entityId
}
