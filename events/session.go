package events

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/cowslice"
	"github.com/openziti/foundation/util/stringz"
	"github.com/pkg/errors"
	"reflect"
)

const SessionEventTypeCreated = "created"
const SessionEventTypeDeleted = "deleted"

type EdgeSessionEvent struct {
	Namespace    string `json:"namespace"`
	EventType    string `json:"event_type"`
	Id           string `json:"id"`
	Token        string `json:"token"`
	ApiSessionId string `json:"api_session_id"`
	IdentityId   string `json:"identity_id"`
}

func (event *EdgeSessionEvent) String() string {
	return fmt.Sprintf("%v.%v id=%v token=%v apiSessionId=%v identityId=%v",
		event.Namespace, event.EventType, event.Id, event.Token, event.ApiSessionId, event.IdentityId)
}

type EdgeSessionEventHandler interface {
	AcceptEdgeSessionEvent(event *EdgeSessionEvent)
}

var sessionEventHandlerRegistry = cowslice.NewCowSlice(make([]EdgeSessionEventHandler, 0))

func getSessionEventHandlers() []EdgeSessionEventHandler {
	return sessionEventHandlerRegistry.Value().([]EdgeSessionEventHandler)
}

func Init(sessionStore persistence.SessionStore) {
	sessionStore.AddListener(boltz.EventCreate, sessionCreated)
	sessionStore.AddListener(boltz.EventDelete, sessionDeleted)
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

	event := &EdgeSessionEvent{
		Namespace:    "edge.sessions",
		EventType:    "created",
		Id:           session.Id,
		Token:        session.Token,
		ApiSessionId: session.ApiSessionId,
		IdentityId:   session.ApiSession.IdentityId,
	}

	for _, handler := range getSessionEventHandlers() {
		go handler.AcceptEdgeSessionEvent(event)
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

	event := &EdgeSessionEvent{
		Namespace: "edge.session",
		EventType: "deleted",
		Id:        session.Id,
		Token:     session.Token,
	}

	for _, handler := range getSessionEventHandlers() {
		go handler.AcceptEdgeSessionEvent(event)
	}
}

func registerSessionEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(EdgeSessionEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/edge/events/EdgeSessionEventHandler interface.", reflect.TypeOf(val))
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
			return errors.Errorf("invalid type %v for edge.sessions include configuration", reflect.TypeOf(includeVar))
		}
	}

	if len(includeList) == 0 || (len(includeList) == 2 && stringz.ContainsAll(includeList, SessionEventTypeCreated, SessionEventTypeDeleted)) {
		AddSessionEventHandler(handler)
	} else {
		for _, include := range includeList {
			if include != SessionEventTypeCreated && include != SessionEventTypeDeleted {
				return errors.Errorf("invalid include %v for edge.sessions. valid values are ['created', 'deleted']", include)
			}
		}

		AddSessionEventHandler(&edgeSessionEventAdapter{
			wrapped:     handler,
			includeList: includeList,
		})
	}

	return nil
}

type edgeSessionEventAdapter struct {
	wrapped     EdgeSessionEventHandler
	includeList []string
}

func (adapter *edgeSessionEventAdapter) AcceptEdgeSessionEvent(event *EdgeSessionEvent) {
	if stringz.Contains(adapter.includeList, event.EventType) {
		adapter.wrapped.AcceptEdgeSessionEvent(event)
	}
}
