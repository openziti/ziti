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

package xt

func NewStrategyChangeEvent(serviceId string, current, added, changed, removed []Terminator) StrategyChangeEvent {
	return &strategyChangeEvent{
		serviceId: serviceId,
		current:   current,
		added:     added,
		changed:   changed,
		removed:   removed,
	}
}

func TList(terminators ...Terminator) []Terminator {
	return terminators
}

type strategyChangeEvent struct {
	serviceId string
	current   []Terminator
	added     []Terminator
	changed   []Terminator
	removed   []Terminator
}

func (event *strategyChangeEvent) GetServiceId() string {
	return event.serviceId
}

func (event *strategyChangeEvent) GetCurrent() []Terminator {
	return event.current
}

func (event *strategyChangeEvent) GetAdded() []Terminator {
	return event.added
}

func (event *strategyChangeEvent) GetChanged() []Terminator {
	return event.changed
}

func (event *strategyChangeEvent) GetRemoved() []Terminator {
	return event.removed
}

func NewDialFailedEvent(terminator Terminator) TerminatorEvent {
	return &defaultEvent{
		terminator: terminator,
		eventType:  eventTypeFailed,
	}
}

func NewDialSucceeded(terminator Terminator) TerminatorEvent {
	return &defaultEvent{
		terminator: terminator,
		eventType:  eventTypeSucceeded,
	}
}

func NewSessionEnded(terminator Terminator) TerminatorEvent {
	return &defaultEvent{
		terminator: terminator,
		eventType:  eventTypeSessionEnded,
	}
}

type eventType int

const (
	eventTypeFailed eventType = iota
	eventTypeSucceeded
	eventTypeSessionEnded
)

type defaultEvent struct {
	terminator Terminator
	eventType  eventType
}

func (event *defaultEvent) GetTerminator() Terminator {
	return event.terminator
}

func (event *defaultEvent) Accept(visitor EventVisitor) {
	if event.eventType == eventTypeFailed {
		visitor.VisitDialFailed(event)
	} else if event.eventType == eventTypeSucceeded {
		visitor.VisitDialSucceeded(event)
	}
}

var _ EventVisitor = DefaultEventVisitor{}

type DefaultEventVisitor struct{}

func (visitor DefaultEventVisitor) VisitDialFailed(TerminatorEvent)    {}
func (visitor DefaultEventVisitor) VisitDialSucceeded(TerminatorEvent) {}
func (visitor DefaultEventVisitor) VisitSessionEnded(TerminatorEvent)  {}
