package events

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/util/cowslice"
	"github.com/openziti/foundation/util/stringz"
	"github.com/pkg/errors"
	"reflect"
	"time"
)

const SessionEventTypeCreated = "created"
const SessionEventTypeDeleted = "deleted"
const SessionEventNS = "edge.sessions"

type SessionEvent struct {
	Namespace    string    `json:"namespace"`
	EventType    string    `json:"event_type"`
	Id           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Token        string    `json:"token"`
	ApiSessionId string    `json:"api_session_id"`
	IdentityId   string    `json:"identity_id"`
}

func (event *SessionEvent) String() string {
	return fmt.Sprintf("%v.%v id=%v timestamp=%v token=%v apiSessionId=%v identityId=%v",
		event.Namespace, event.EventType, event.Id, event.Timestamp, event.Token, event.ApiSessionId, event.IdentityId)
}

type SessionEventHandler interface {
	AcceptSessionEvent(event *SessionEvent)
}

var sessionEventHandlerRegistry = cowslice.NewCowSlice(make([]SessionEventHandler, 0))

func getSessionEventHandlers() []SessionEventHandler {
	return sessionEventHandlerRegistry.Value().([]SessionEventHandler)
}

func sessionCreated(args ...interface{}) {
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
		Timestamp:    time.Now(),
		Token:        session.Token,
		ApiSessionId: session.ApiSessionId,
		IdentityId:   session.ApiSession.IdentityId,
	}

	for _, handler := range getSessionEventHandlers() {
		go handler.AcceptSessionEvent(event)
	}
}

func sessionDeleted(args ...interface{}) {
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
		Namespace: SessionEventNS,
		EventType: SessionEventTypeDeleted,
		Id:        session.Id,
		Timestamp: time.Now(),
		Token:     session.Token,
	}

	for _, handler := range getSessionEventHandlers() {
		go handler.AcceptSessionEvent(event)
	}
}

func registerSessionEventHandler(val interface{}, config map[interface{}]interface{}) error {
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
		AddSessionEventHandler(handler)
	} else {
		for _, include := range includeList {
			if include != SessionEventTypeCreated && include != SessionEventTypeDeleted {
				return errors.Errorf("invalid include %v for %v. valid values are ['created', 'deleted']", include, SessionEventNS)
			}
		}

		AddSessionEventHandler(&sessionEventAdapter{
			wrapped:     handler,
			includeList: includeList,
		})
	}

	return nil
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
