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
	"github.com/openziti/fabric/controller/event"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/metrics/metrics_pb"
	"strings"
)

type ctrlChannelMetricsMapper struct{}

func (ctrlChannelMetricsMapper) mapMetrics(_ *metrics_pb.MetricsMessage, event *event.MetricsEvent) {
	if strings.HasPrefix(event.Metric, "ctrl.") {
		parts := strings.Split(event.Metric, ":")
		event.Metric = parts[0]
		event.SourceEntityId = parts[1]
	}
}

type linkMetricsMapper struct {
	network *network.Network
}

func (self *linkMetricsMapper) mapMetrics(_ *metrics_pb.MetricsMessage, event *event.MetricsEvent) {
	if strings.HasPrefix(event.Metric, "link.") {
		var name, linkId string
		if parts := strings.Split(event.Metric, ":"); len(parts) == 2 {
			name = parts[0]
			linkId = parts[1]
		} else if strings.HasSuffix(event.Metric, "latency") || strings.HasSuffix(event.Metric, "queue_time") {
			name, linkId = ExtractId(event.Metric, "link.", 1)
		} else {
			name, linkId = ExtractId(event.Metric, "link.", 2)
		}
		event.Metric = name
		event.SourceEntityId = linkId

		if link, _ := self.network.GetLink(linkId); link != nil {
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

func ExtractId(name string, prefix string, suffixLen int) (string, string) {
	rest := strings.TrimPrefix(name, prefix)
	vals := strings.Split(rest, ".")
	idVals := vals[:len(vals)-suffixLen]
	entityId := strings.Join(idVals, ".")
	return prefix + rest[len(entityId)+1:], entityId
}
