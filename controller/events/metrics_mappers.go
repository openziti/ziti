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
	"strings"

	"github.com/openziti/ziti/v2/common/servermetrics/metrics_pb"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/model"
)

// splitMetricName returns the name a metric is emitted under and the entity id its raw name carries.
//
// Pure string work, deliberately: it is what the built-in mappers use to rewrite a name, and also what the
// metric filter's fast path asks so it can tell what a filter written against emitted names would see. That
// path runs for every metric in every message, so it cannot afford the per-link lookup the link mapper's tag
// enrichment does, and does not need it.
func splitMetricName(raw string) (string, string) {
	if strings.HasPrefix(raw, "ctrl.") {
		if parts := strings.Split(raw, ":"); len(parts) > 1 {
			return parts[0], parts[1]
		}
		return raw, ""
	}

	if strings.HasPrefix(raw, "link.") {
		if parts := strings.Split(raw, ":"); len(parts) == 2 {
			return parts[0], parts[1]
		}
		if strings.HasSuffix(raw, "latency") || strings.HasSuffix(raw, "queue_time") {
			return ExtractId(raw, "link.", 1)
		}
		return ExtractId(raw, "link.", 2)
	}

	return raw, ""
}

// mapMetricName returns the name a metric is emitted under, for callers that do not need the entity id.
func mapMetricName(raw string) string {
	name, _ := splitMetricName(raw)
	return name
}

// addBuiltInMetricsMappers registers the mappers every dispatcher runs. getLink looks up a link by id for the
// link mapper's tags, returning nil for a link it does not know.
func (self *Dispatcher) addBuiltInMetricsMappers(getLink func(string) (*model.Link, bool)) {
	// Control channel metrics, which the controller keeps per router as ctrl.<metric>:<routerId>. The router id
	// moves to the source entity, leaving ctrl.<metric>.
	self.addMetricsMapper(func(_ *metrics_pb.MetricsMessage, event *event.MetricsEvent) {
		if strings.HasPrefix(event.Metric, "ctrl.") {
			event.Metric, event.SourceEntityId = splitMetricName(event.Metric)
		}
	})

	// Link metrics, which routers report per link as link.<linkId>.<metric>, or link.<metric>:<linkId> for the
	// drop meters. The link id moves to the source entity, and a link this controller knows is tagged with its
	// two routers.
	self.addMetricsMapper(func(_ *metrics_pb.MetricsMessage, event *event.MetricsEvent) {
		if strings.HasPrefix(event.Metric, "link.") {
			name, linkId := splitMetricName(event.Metric)
			event.Metric = name
			event.SourceEntityId = linkId

			if link, _ := getLink(linkId); link != nil {
				sourceTags := event.Tags
				event.Tags = map[string]string{}
				for k, v := range sourceTags {
					event.Tags[k] = v
				}
				event.Tags["sourceRouterId"] = link.GetSrc().Id
				event.Tags["targetRouterId"] = link.DstId
			}
		}
	})
}

// ExtractId splits a metric name carrying an entity id into the name it is emitted under and that id.
// prefix is the namespace the name opens with, suffixLen is how many trailing dot-separated segments name
// the metric itself, and everything between the two is the id, dots included.
//
// A name with no segments to spare for an id is returned unchanged, with an empty id. Callers get that for
// any name in the namespace that does not carry one, so the namespace is safe to use for metrics that have
// no entity.
func ExtractId(name string, prefix string, suffixLen int) (string, string) {
	rest := strings.TrimPrefix(name, prefix)
	vals := strings.Split(rest, ".")
	if len(vals) <= suffixLen {
		return name, ""
	}
	idVals := vals[:len(vals)-suffixLen]
	entityId := strings.Join(idVals, ".")
	return prefix + rest[len(entityId)+1:], entityId
}
