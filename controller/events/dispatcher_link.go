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
	"github.com/openziti/ziti/controller/event"
	"github.com/pkg/errors"
	"reflect"
)

func (self *Dispatcher) AddLinkEventHandler(handler event.LinkEventHandler) {
	self.linkEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveLinkEventHandler(handler event.LinkEventHandler) {
	self.linkEventHandlers.DeleteIf(func(val event.LinkEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.LinkEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) AcceptLinkEvent(event *event.LinkEvent) {
	go func() {
		for _, handler := range self.linkEventHandlers.Value() {
			handler.AcceptLinkEvent(event)
		}
	}()
}

func (self *Dispatcher) registerLinkEventHandler(eventType string, val interface{}, _ map[string]interface{}) error {
	handler, ok := val.(event.LinkEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/event/LinkEventHandler interface.", reflect.TypeOf(val))
	}

	if eventType != event.LinkEventNS {
		handler = &linkEventOldNsAdapter{
			namespace: eventType,
			wrapped:   handler,
		}
	}
	self.AddLinkEventHandler(handler)

	return nil
}

func (self *Dispatcher) unregisterLinkEventHandler(val interface{}) {
	if handler, ok := val.(event.LinkEventHandler); ok {
		self.RemoveLinkEventHandler(handler)
	}
}

type linkEventOldNsAdapter struct {
	namespace string
	wrapped   event.LinkEventHandler
}

func (self *linkEventOldNsAdapter) AcceptLinkEvent(evt *event.LinkEvent) {
	nsEvent := *evt
	nsEvent.Namespace = self.namespace
	self.wrapped.AcceptLinkEvent(&nsEvent)
}

func (self *linkEventOldNsAdapter) IsWrapping(value event.LinkEventHandler) bool {
	if self.wrapped == value {
		return true
	}
	if w, ok := self.wrapped.(event.LinkEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}
