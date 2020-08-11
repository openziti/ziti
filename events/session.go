package events

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/cowslice"
)

type SessionCreatedEvent struct {
	Id           string
	Token        string
	ApiSessionId string
	IdentityId   string
}

type SessionDeletedEvent struct {
	Id    string
	Token string
}

type SessionEventHandler interface {
	HandleSessionCreated(event *SessionCreatedEvent)
	HandleSessionDeleted(event *SessionDeletedEvent)
}

var sessionEventHandlerRegistry = cowslice.NewCowSlice(make([]SessionEventHandler, 0))

func getSessionEventHandlers() []SessionEventHandler {
	return sessionEventHandlerRegistry.Value().([]SessionEventHandler)
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

	event := &SessionCreatedEvent{
		Id:           session.Id,
		Token:        session.Token,
		ApiSessionId: session.ApiSessionId,
		IdentityId:   session.ApiSession.IdentityId,
	}

	for _, handler := range getSessionEventHandlers() {
		go handler.HandleSessionCreated(event)
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

	event := &SessionDeletedEvent{
		Id:    session.Id,
		Token: session.Token,
	}

	for _, handler := range getSessionEventHandlers() {
		go handler.HandleSessionDeleted(event)
	}
}
