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

package api_impl

import (
	"encoding/json"
	"fmt"
	"github.com/openziti/fabric/controller/event"
	"github.com/openziti/fabric/controller/events"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/metrics/metrics_pb"
	"github.com/pkg/errors"
	"strings"
)

const EntityNameMetrics = "metrics"

type MetricsModelMapper interface {
	MapInspectResultToMetricsResult(inspectResult *network.InspectResult) (*string, error)
	MapInspectResultValueToMetricsResult(inspectResultValue *network.InspectResultValue) (any, error)
}

type metricsResultMapper struct {
	network           *network.Network
	format            string
	includeTimestamps bool
}

func NewMetricsModelMapper(n *network.Network, format string, includeTimestamps bool) MetricsModelMapper {
	return &metricsResultMapper{
		network:           n,
		format:            format,
		includeTimestamps: includeTimestamps,
	}
}

func (self *metricsResultMapper) MapInspectResultValueToMetricsResult(inspectResultValue *network.InspectResultValue) (any, error) {
	var result any

	msg := &metrics_pb.MetricsMessage{}
	if err := json.Unmarshal([]byte(inspectResultValue.Value), msg); err == nil {

		var metricEvents []event.MetricsEvent

		adapter := self.network.GetEventDispatcher().NewFilteredMetricsAdapter(nil, nil, event.MetricsEventHandlerF(func(event *event.MetricsEvent) {
			metricEvents = append(metricEvents, *event)
		}))

		adapter.AcceptMetricsMsg(msg)

		switch self.format {
		case "json":
			result = metricEvents
		case "prometheus":
			var promMsgs []string

			for _, msg := range metricEvents {
				event := (events.PrometheusMetricsEvent)(msg)
				o, err := event.Marshal(self.includeTimestamps)

				if err == nil {
					promMsgs = append(promMsgs, string(o))
				} else {
					promMsgs = append(promMsgs, fmt.Sprint(err))
				}
			}
			result = promMsgs
		default:
			return nil, errors.New(fmt.Sprintf("Unsupported metrics format %s requested", self.format))
		}
	} else {
		return nil, err
	}
	return result, nil
}

func (self *metricsResultMapper) MapInspectResultToMetricsResult(inspectResult *network.InspectResult) (*string, error) {

	var emit string

	var r []any

	for _, val := range inspectResult.Results {
		s, _ := self.MapInspectResultValueToMetricsResult(val)

		r = append(r, s)
	}

	switch self.format {
	case "json":
		var js []any
		for _, mg := range r {
			for _, m := range mg.([]event.MetricsEvent) {
				js = append(js, m)
			}
		}
		s, err := json.Marshal(js)
		if err != nil {
			return nil, err
		}
		emit = string(s)
	case "prometheus":
		var prom string

		for _, m := range r {
			prom += strings.Join(m.([]string), "")
		}
		emit = prom
	}

	return &emit, nil
}
