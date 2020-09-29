package events

import (
	"github.com/openziti/foundation/events"
	"github.com/openziti/foundation/metrics"
	"github.com/pkg/errors"
	"reflect"
)

func registerMetricsEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(metrics.Handler)
	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/foundation/metrics/Handler interface.", reflect.TypeOf(val))
	}

	var sourceFilter = ""
	if sourceRegexVal, ok := config["sourceFilter"]; ok {
		sourceFilter, ok = sourceRegexVal.(string)
		if !ok {
			return errors.Errorf("invalid sourceFilter value %v of type %v. must be string", sourceRegexVal, reflect.TypeOf(sourceRegexVal))
		}
	}

	var metricFilter = ""
	if metricRegexVal, ok := config["metricFilter"]; ok {
		metricFilter, ok = metricRegexVal.(string)
		if !ok {
			return errors.Errorf("invalid metricFilter value %v of type %v. must be string", metricRegexVal, reflect.TypeOf(metricRegexVal))
		}
	}

	events.AddMetricsEventHandler(handler)
	return nil
}
