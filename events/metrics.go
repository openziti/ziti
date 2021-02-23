package events

import (
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/uuid"
	"github.com/openziti/foundation/events"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/pkg/errors"
	"reflect"
	"regexp"
	"strings"
)

func init() {
	AddMetricsNameMapper(mapLinkIds)
	AddMetricsNameMapper(mapCtrlIds)
}

func mapLinkIds(name string) (string, string, bool) {
	if strings.HasPrefix(name, "link.") {
		return ExtractId(name, "link.", 2)
	}
	return "", "", false
}

func mapCtrlIds(name string) (string, string, bool) {
	if strings.HasPrefix(name, "ctrl.") {
		return ExtractId(name, "ctrl.", 2)
	}
	return "", "", false
}

type MetricsNameMapper func(name string) (string, string, bool)

var metricsNameMappers []MetricsNameMapper

func AddMetricsNameMapper(mapper MetricsNameMapper) {
	metricsNameMappers = append(metricsNameMappers, mapper)
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

	events.AddMetricsEventHandler(adapter)
	return nil
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
	}

	for _, mapper := range metricsNameMappers {
		if mappedName, entityId, mapped := mapper(name); mapped {
			result.Metric = mappedName
			result.SourceEntityId = entityId
			break
		}
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
		adapter.filterMetric(".count", value.Count, event)
		adapter.filterMetric(".mean_rate", value.MeanRate, event)
		adapter.filterMetric(".m1_rate", value.M1Rate, event)
		adapter.filterMetric(".m5_rate", value.M5Rate, event)
		adapter.filterMetric(".m15_rate", value.M15Rate, event)
		adapter.finishEvent(event)
	}

	for name, value := range msg.Histograms {
		event := adapter.newMetricEvent(msg, name, parentEventId)
		adapter.filterMetric(".count", value.Count, event)
		adapter.filterMetric(".min", value.Min, event)
		adapter.filterMetric(".max", value.Max, event)
		adapter.filterMetric(".mean", value.Mean, event)
		adapter.filterMetric(".std_dev", value.StdDev, event)
		adapter.filterMetric(".variance", value.Variance, event)
		adapter.filterMetric(".p50", value.P50, event)
		adapter.filterMetric(".p75", value.P75, event)
		adapter.filterMetric(".p95", value.P95, event)
		adapter.filterMetric(".p99", value.P99, event)
		adapter.filterMetric(".p999", value.P999, event)
		adapter.filterMetric(".p9999", value.P9999, event)
		adapter.finishEvent(event)
	}

	for name, value := range msg.Timers {
		event := adapter.newMetricEvent(msg, name, parentEventId)
		adapter.filterMetric(".count", value.Count, event)

		adapter.filterMetric(".mean_rate", value.MeanRate, event)
		adapter.filterMetric(".m1_rate", value.M1Rate, event)
		adapter.filterMetric(".m5_rate", value.M5Rate, event)
		adapter.filterMetric(".m15_rate", value.M15Rate, event)

		adapter.filterMetric(".min", value.Min, event)
		adapter.filterMetric(".max", value.Max, event)
		adapter.filterMetric(".mean", value.Mean, event)
		adapter.filterMetric(".std_dev", value.StdDev, event)
		adapter.filterMetric(".variance", value.Variance, event)
		adapter.filterMetric(".p50", value.P50, event)
		adapter.filterMetric(".p75", value.P75, event)
		adapter.filterMetric(".p95", value.P95, event)
		adapter.filterMetric(".p99", value.P99, event)
		adapter.filterMetric(".p999", value.P999, event)
		adapter.filterMetric(".p9999", value.P9999, event)
		adapter.finishEvent(event)
	}
}

func (adapter *metricsAdapter) filterMetric(suffix string, value interface{}, event *MetricsEvent) {
	name := event.Metric + suffix
	if adapter.nameMatches(name) {
		if event.Metrics == nil {
			event.Metrics = make(map[string]interface{})
		}
		event.Metrics[name] = value
	}
}

func (adapter *metricsAdapter) nameMatches(name string) bool {
	return adapter.metricFilter == nil || adapter.metricFilter.Match([]byte(name))
}

type MetricsEvent struct {
	Namespace      string
	SourceAppId    string
	SourceEntityId string
	Timestamp      *timestamp.Timestamp
	Metric         string
	Metrics        map[string]interface{}
	Tags           map[string]string
	SourceEventId  string
}

type MetricsEventHandler interface {
	AcceptMetricsEvent(event *MetricsEvent)
}

func ExtractId(name string, prefix string, suffixLen int) (string, string, bool) {
	rest := strings.TrimPrefix(name, prefix)
	vals := strings.Split(rest, ".")
	idVals := vals[:len(vals)-suffixLen]
	entityId := strings.Join(idVals, ".")
	return prefix + rest[len(entityId)+1:], entityId, true
}
