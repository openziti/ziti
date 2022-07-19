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

package event

import (
	"github.com/openziti/metrics/metrics_pb"
	"regexp"
)

var _ Dispatcher = DispatcherMock{}

type DispatcherMock struct{}

func (d DispatcherMock) RegisterEventType(string, RegistrationHandler) {}

func (d DispatcherMock) RegisterEventHandlerFactory(string, HandlerFactory) {}

func (d DispatcherMock) Dispatch(Event) {}

func (d DispatcherMock) AddCircuitEventHandler(CircuitEventHandler) {}

func (d DispatcherMock) RemoveCircuitEventHandler(CircuitEventHandler) {}

func (d DispatcherMock) AddLinkEventHandler(LinkEventHandler) {}

func (d DispatcherMock) RemoveLinkEventHandler(LinkEventHandler) {}

func (d DispatcherMock) AddMetricsMapper(MetricsMapper) {}

func (d DispatcherMock) AddMetricsEventHandler(MetricsEventHandler) {}

func (d DispatcherMock) RemoveMetricsEventHandler(MetricsEventHandler) {}

func (d DispatcherMock) AddMetricsMessageHandler(MetricsMessageHandler) {}

func (d DispatcherMock) RemoveMetricsMessageHandler(MetricsMessageHandler) {}

func (d DispatcherMock) NewFilteredMetricsAdapter(*regexp.Regexp, *regexp.Regexp, MetricsEventHandler) MetricsMessageHandler {
	return nil
}

func (d DispatcherMock) AddRouterEventHandler(RouterEventHandler) {}

func (d DispatcherMock) RemoveRouterEventHandler(RouterEventHandler) {}

func (d DispatcherMock) AddServiceEventHandler(ServiceEventHandler) {}

func (d DispatcherMock) RemoveServiceEventHandler(ServiceEventHandler) {}

func (d DispatcherMock) AddTerminatorEventHandler(TerminatorEventHandler) {}

func (d DispatcherMock) RemoveTerminatorEventHandler(TerminatorEventHandler) {}

func (d DispatcherMock) AddUsageEventHandler(UsageEventHandler) {}

func (d DispatcherMock) RemoveUsageEventHandler(UsageEventHandler) {}

func (d DispatcherMock) AcceptCircuitEvent(*CircuitEvent) {}

func (d DispatcherMock) AcceptLinkEvent(*LinkEvent) {}

func (d DispatcherMock) AcceptMetricsEvent(*MetricsEvent) {}

func (d DispatcherMock) AcceptMetricsMsg(*metrics_pb.MetricsMessage) {}

func (d DispatcherMock) AcceptRouterEvent(*RouterEvent) {}

func (d DispatcherMock) AcceptServiceEvent(*ServiceEvent) {}

func (d DispatcherMock) AcceptTerminatorEvent(*TerminatorEvent) {}

func (d DispatcherMock) AcceptUsageEvent(*UsageEvent) {}
