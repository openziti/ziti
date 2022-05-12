/*
	Copyright NetFoundry, Inc.

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
	"github.com/openziti/fabric/event"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/metrics/metrics_pb"
	"github.com/openziti/foundation/util/concurrenz"
)

var metricsEventHandlerRegistry = &concurrenz.CopyOnWriteSlice[MessageHandler]{}

func getMetricsEventHandlers() []MessageHandler {
	return metricsEventHandlerRegistry.Value()
}

func AddMetricsEventHandler(handler MessageHandler) {
	metricsEventHandlerRegistry.Append(handler)
}

func RemoveMetricsEventHandler(handler MessageHandler) {
	metricsEventHandlerRegistry.Delete(handler)
}

// MessageHandler represents a sink for metric events
type MessageHandler interface {
	// AcceptMetrics is called when new metrics become available
	AcceptMetrics(message *metrics_pb.MetricsMessage)
}

type eventWrapper struct {
	msg *metrics_pb.MetricsMessage
}

func (e *eventWrapper) Handle() {
	for _, handler := range getMetricsEventHandlers() {
		handler.AcceptMetrics(e.msg)
	}
}

func NewDispatchWrapper(delegate func(event event.Event)) MessageHandler {
	return &eventDispatcherWrapper{delegate: delegate}
}

type eventDispatcherWrapper struct {
	delegate func(event event.Event)
}

func (dispatcherWrapper *eventDispatcherWrapper) AcceptMetrics(message *metrics_pb.MetricsMessage) {
	eventWrapper := &eventWrapper{msg: message}
	dispatcherWrapper.delegate(eventWrapper)
}

func InitMetricHandlers(cfg *metrics.Config) {
	if cfg != nil {
		for _, handler := range cfg.Handlers {
			metricsEventHandlerRegistry.Append(handler)
		}
	}
}
