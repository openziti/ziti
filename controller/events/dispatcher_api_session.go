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
	"fmt"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/event"
	"github.com/pkg/errors"
	"reflect"
	"time"
)

func (self *Dispatcher) AddApiSessionEventHandler(handler event.ApiSessionEventHandler) {
	self.apiSessionEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveApiSessionEventHandler(handler event.ApiSessionEventHandler) {
	self.apiSessionEventHandlers.DeleteIf(func(val event.ApiSessionEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.ApiSessionEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) initApiSessionEvents(stores *db.Stores) {
	stores.ApiSession.AddEntityEventListenerF(self.apiSessionCreated, boltz.EntityCreated)
	stores.ApiSession.AddEntityEventListenerF(self.apiSessionDeleted, boltz.EntityDeleted)
}

func (self *Dispatcher) AcceptApiSessionEvent(evt *event.ApiSessionEvent) {
	for _, handler := range self.apiSessionEventHandlers.Value() {
		go handler.AcceptApiSessionEvent(evt)
	}
}

func (self *Dispatcher) apiSessionCreated(apiSession *db.ApiSession) {
	evt := &event.ApiSessionEvent{
		Namespace:  event.ApiSessionEventNS,
		EventType:  event.ApiSessionEventTypeCreated,
		EventSrcId: self.ctrlId,
		Id:         apiSession.Id,
		Type:       event.ApiSessionTypeLegacy,
		Timestamp:  time.Now(),
		Token:      apiSession.Token,
		IdentityId: apiSession.IdentityId,
		IpAddress:  apiSession.IPAddress,
	}

	self.AcceptApiSessionEvent(evt)
}

func (self *Dispatcher) apiSessionDeleted(apiSession *db.ApiSession) {
	evt := &event.ApiSessionEvent{
		Namespace:  event.ApiSessionEventNS,
		EventType:  event.ApiSessionEventTypeDeleted,
		EventSrcId: self.ctrlId,
		Id:         apiSession.Id,
		Type:       event.ApiSessionTypeLegacy,
		Timestamp:  time.Now(),
		Token:      apiSession.Token,
		IdentityId: apiSession.IdentityId,
		IpAddress:  apiSession.IPAddress,
	}

	self.AcceptApiSessionEvent(evt)
}

func (self *Dispatcher) registerApiSessionEventHandler(eventType string, val interface{}, config map[string]interface{}) error {
	handler, ok := val.(event.ApiSessionEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/ziti/controller/event/ApiSessionEventHandler interface.", reflect.TypeOf(val))
	}

	if eventType != event.ApiSessionEventNS {
		handler = &apiSessionEventOldNsAdapter{
			namespace: eventType,
			wrapped:   handler,
		}
	}

	var includeList []string
	if includeVar, ok := config["include"]; ok {
		if includeStr, ok := includeVar.(string); ok {
			includeList = append(includeList, includeStr)
		} else if includeIntfList, ok := includeVar.([]interface{}); ok {
			for _, val := range includeIntfList {
				includeList = append(includeList, fmt.Sprintf("%v", val))
			}
		} else {
			return errors.Errorf("invalid type %v for %v include configuration", reflect.TypeOf(includeVar), event.ApiSessionEventNS)
		}
	}

	if len(includeList) == 0 || (len(includeList) == 2 && stringz.ContainsAll(includeList, event.ApiSessionEventTypeCreated, event.ApiSessionEventTypeDeleted)) {
		self.AddApiSessionEventHandler(handler)
	} else {
		for _, include := range includeList {
			if include != event.ApiSessionEventTypeCreated && include != event.ApiSessionEventTypeDeleted {
				return errors.Errorf("invalid include %v for %v. valid values are ['created', 'deleted']", include, event.ApiSessionEventNS)
			}
		}

		self.AddApiSessionEventHandler(&apiSessionEventAdapter{
			wrapped:     handler,
			includeList: includeList,
		})
	}

	return nil
}

func (self *Dispatcher) unregisterApiSessionEventHandler(val interface{}) {
	if handler, ok := val.(event.ApiSessionEventHandler); ok {
		self.RemoveApiSessionEventHandler(handler)
	}
}

type apiSessionEventAdapter struct {
	wrapped     event.ApiSessionEventHandler
	includeList []string
}

func (adapter *apiSessionEventAdapter) AcceptApiSessionEvent(evt *event.ApiSessionEvent) {
	if stringz.Contains(adapter.includeList, evt.EventType) {
		adapter.wrapped.AcceptApiSessionEvent(evt)
	}
}

func (self *apiSessionEventAdapter) IsWrapping(value event.ApiSessionEventHandler) bool {
	if self.wrapped == value {
		return true
	}
	if w, ok := self.wrapped.(event.ApiSessionEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}

type apiSessionEventOldNsAdapter struct {
	namespace string
	wrapped   event.ApiSessionEventHandler
}

func (self *apiSessionEventOldNsAdapter) AcceptApiSessionEvent(evt *event.ApiSessionEvent) {
	nsEvent := *evt
	nsEvent.Namespace = self.namespace
	self.wrapped.AcceptApiSessionEvent(&nsEvent)
}

func (self *apiSessionEventOldNsAdapter) IsWrapping(value event.ApiSessionEventHandler) bool {
	if self.wrapped == value {
		return true
	}
	if w, ok := self.wrapped.(event.ApiSessionEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}
