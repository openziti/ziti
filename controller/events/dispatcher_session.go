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

func (self *Dispatcher) AddSessionEventHandler(handler event.SessionEventHandler) {
	self.sessionEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveSessionEventHandler(handler event.SessionEventHandler) {
	self.sessionEventHandlers.DeleteIf(func(val event.SessionEventHandler) bool {
		if val == handler {
			return true
		}
		if w, ok := val.(event.SessionEventHandlerWrapper); ok {
			return w.IsWrapping(handler)
		}
		return false
	})
}

func (self *Dispatcher) initSessionEvents(stores *db.Stores) {
	stores.Session.AddEntityEventListenerF(self.sessionCreated, boltz.EntityCreated)
	stores.Session.AddEntityEventListenerF(self.sessionDeleted, boltz.EntityDeleted)
}

func (self *Dispatcher) sessionCreated(session *db.Session) {
	evt := &event.SessionEvent{
		Namespace:    event.SessionEventNS,
		EventType:    event.SessionEventTypeCreated,
		Id:           session.Id,
		SessionType:  session.Type,
		Timestamp:    time.Now(),
		Token:        session.Token,
		ApiSessionId: session.ApiSessionId,
		IdentityId:   session.IdentityId,
		ServiceId:    session.ServiceId,
	}

	for _, handler := range self.sessionEventHandlers.Value() {
		go handler.AcceptSessionEvent(evt)
	}
}

func (self *Dispatcher) sessionDeleted(session *db.Session) {
	evt := &event.SessionEvent{
		Namespace:    event.SessionEventNS,
		EventType:    event.SessionEventTypeDeleted,
		Id:           session.Id,
		SessionType:  session.Type,
		Timestamp:    time.Now(),
		Token:        session.Token,
		ApiSessionId: session.ApiSessionId,
		ServiceId:    session.ServiceId,
	}

	for _, handler := range self.sessionEventHandlers.Value() {
		go handler.AcceptSessionEvent(evt)
	}
}

func (self *Dispatcher) registerSessionEventHandler(val interface{}, config map[string]interface{}) error {
	handler, ok := val.(event.SessionEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/edge/events/SessionEventHandler interface.", reflect.TypeOf(val))
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
			return errors.Errorf("invalid type %v for %v include configuration", reflect.TypeOf(includeVar), event.SessionEventNS)
		}
	}

	if len(includeList) == 0 || (len(includeList) == 2 && stringz.ContainsAll(includeList, event.SessionEventTypeCreated, event.SessionEventTypeDeleted)) {
		self.AddSessionEventHandler(handler)
	} else {
		for _, include := range includeList {
			if include != event.SessionEventTypeCreated && include != event.SessionEventTypeDeleted {
				return errors.Errorf("invalid include %v for %v. valid values are ['created', 'deleted']", include, event.SessionEventNS)
			}
		}

		self.AddSessionEventHandler(&sessionEventAdapter{
			wrapped:     handler,
			includeList: includeList,
		})
	}

	return nil
}

func (self *Dispatcher) unregisterSessionEventHandler(val interface{}) {
	if handler, ok := val.(event.SessionEventHandler); ok {
		self.RemoveSessionEventHandler(handler)
	}
}

type sessionEventAdapter struct {
	wrapped     event.SessionEventHandler
	includeList []string
}

func (adapter *sessionEventAdapter) AcceptSessionEvent(event *event.SessionEvent) {
	if stringz.Contains(adapter.includeList, event.EventType) {
		adapter.wrapped.AcceptSessionEvent(event)
	}
}

func (self *sessionEventAdapter) IsWrapping(value event.SessionEventHandler) bool {
	if self.wrapped == value {
		return true
	}
	if w, ok := self.wrapped.(event.SessionEventHandlerWrapper); ok {
		return w.IsWrapping(value)
	}
	return false
}
