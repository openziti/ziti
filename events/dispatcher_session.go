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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/storage/boltz"
	"github.com/pkg/errors"
	"reflect"
	"time"
)

func (self *Dispatcher) AddSessionEventHandler(handler SessionEventHandler) {
	self.sessionEventHandlers.Append(handler)
}

func (self *Dispatcher) RemoveSessionEventHandler(handler SessionEventHandler) {
	self.sessionEventHandlers.Delete(handler)
}

func (self *Dispatcher) initSessionEvents(stores *persistence.Stores) {
	stores.Session.AddListener(boltz.EventCreate, self.sessionCreated)
	stores.Session.AddListener(boltz.EventDelete, self.sessionDeleted)
}

func (self *Dispatcher) sessionCreated(args ...interface{}) {
	var session *persistence.Session
	if len(args) == 1 {
		session, _ = args[0].(*persistence.Session)
	}

	if session == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	event := &SessionEvent{
		Namespace:    SessionEventNS,
		EventType:    SessionEventTypeCreated,
		Id:           session.Id,
		SessionType:  session.Type,
		Timestamp:    time.Now(),
		Token:        session.Token,
		ApiSessionId: session.ApiSessionId,
		IdentityId:   session.ApiSession.IdentityId,
		ServiceId:    session.ServiceId,
	}

	for _, handler := range self.sessionEventHandlers.Value() {
		go handler.AcceptSessionEvent(event)
	}
}

func (self *Dispatcher) sessionDeleted(args ...interface{}) {
	var session *persistence.Session
	if len(args) == 1 {
		session, _ = args[0].(*persistence.Session)
	}

	if session == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	event := &SessionEvent{
		Namespace:    SessionEventNS,
		EventType:    SessionEventTypeDeleted,
		Id:           session.Id,
		SessionType:  session.Type,
		Timestamp:    time.Now(),
		Token:        session.Token,
		ApiSessionId: session.ApiSessionId,
		ServiceId:    session.ServiceId,
	}

	for _, handler := range self.sessionEventHandlers.Value() {
		go handler.AcceptSessionEvent(event)
	}
}

func (self *Dispatcher) registerSessionEventHandler(val interface{}, config map[string]interface{}) error {
	handler, ok := val.(SessionEventHandler)

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
			return errors.Errorf("invalid type %v for %v include configuration", reflect.TypeOf(includeVar), SessionEventNS)
		}
	}

	if len(includeList) == 0 || (len(includeList) == 2 && stringz.ContainsAll(includeList, SessionEventTypeCreated, SessionEventTypeDeleted)) {
		self.AddSessionEventHandler(handler)
	} else {
		for _, include := range includeList {
			if include != SessionEventTypeCreated && include != SessionEventTypeDeleted {
				return errors.Errorf("invalid include %v for %v. valid values are ['created', 'deleted']", include, SessionEventNS)
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
	if handler, ok := val.(SessionEventHandler); ok {
		self.RemoveSessionEventHandler(handler)
	}
}

type sessionEventAdapter struct {
	wrapped     SessionEventHandler
	includeList []string
}

func (adapter *sessionEventAdapter) AcceptSessionEvent(event *SessionEvent) {
	if stringz.Contains(adapter.includeList, event.EventType) {
		adapter.wrapped.AcceptSessionEvent(event)
	}
}
