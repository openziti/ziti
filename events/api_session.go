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

const ApiSessionEventTypeCreated = "created"
const ApiSessionEventTypeDeleted = "deleted"
const ApiSessionEventNS = "edge.apiSessions"

type ApiSessionEvent struct {
	Namespace  string    `json:"namespace"`
	EventType  string    `json:"event_type"`
	Id         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Token      string    `json:"token"`
	IdentityId string    `json:"identity_id"`
	IpAddress  string    `json:"ip_address"`
}

func (event *ApiSessionEvent) String() string {
	return fmt.Sprintf("%v.%v id=%v timestamp=%v token=%v identityId=%v ipAddress=%v",
		event.Namespace, event.EventType, event.Id, event.Timestamp, event.Token, event.IdentityId, event.IpAddress)
}

type ApiSessionEventHandler interface {
	AcceptApiSessionEvent(event *ApiSessionEvent)
}

var apiSessionEventHandlerRegistry = cowslice.NewCowSlice(make([]ApiSessionEventHandler, 0))

func getApiSessionEventHandlers() []ApiSessionEventHandler {
	return apiSessionEventHandlerRegistry.Value().([]ApiSessionEventHandler)
}

func apiSessionCreated(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	}

	if apiSession == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	event := &ApiSessionEvent{
		Namespace:  ApiSessionEventNS,
		EventType:  ApiSessionEventTypeCreated,
		Id:         apiSession.Id,
		Timestamp:  time.Now(),
		Token:      apiSession.Token,
		IdentityId: apiSession.IdentityId,
		IpAddress:  apiSession.IPAddress,
	}

	for _, handler := range getApiSessionEventHandlers() {
		go handler.AcceptApiSessionEvent(event)
	}
}

func apiSessionDeleted(args ...interface{}) {
	var apiSession *persistence.ApiSession
	if len(args) == 1 {
		apiSession, _ = args[0].(*persistence.ApiSession)
	}

	if apiSession == nil {
		log := pfxlog.Logger()
		log.Error("could not cast event args to event details")
		return
	}

	event := &ApiSessionEvent{
		Namespace:  ApiSessionEventNS,
		EventType:  ApiSessionEventTypeDeleted,
		Id:         apiSession.Id,
		Timestamp:  time.Now(),
		Token:      apiSession.Token,
		IdentityId: apiSession.IdentityId,
		IpAddress:  apiSession.IPAddress,
	}

	for _, handler := range getApiSessionEventHandlers() {
		go handler.AcceptApiSessionEvent(event)
	}
}

func registerApiSessionEventHandler(val interface{}, config map[interface{}]interface{}) error {
	handler, ok := val.(ApiSessionEventHandler)

	if !ok {
		return errors.Errorf("type %v doesn't implement github.com/openziti/edge/events/ApiSessionEventHandler interface.", reflect.TypeOf(val))
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
			return errors.Errorf("invalid type %v for %v include configuration", reflect.TypeOf(includeVar), ApiSessionEventNS)
		}
	}

	if len(includeList) == 0 || (len(includeList) == 2 && stringz.ContainsAll(includeList, ApiSessionEventTypeCreated, ApiSessionEventTypeDeleted)) {
		AddApiSessionEventHandler(handler)
	} else {
		for _, include := range includeList {
			if include != ApiSessionEventTypeCreated && include != ApiSessionEventTypeDeleted {
				return errors.Errorf("invalid include %v for %v. valid values are ['created', 'deleted']", include, ApiSessionEventNS)
			}
		}

		AddApiSessionEventHandler(&ApiSessionEventAdapter{
			wrapped:     handler,
			includeList: includeList,
		})
	}

	return nil
}

type ApiSessionEventAdapter struct {
	wrapped     ApiSessionEventHandler
	includeList []string
}

func (adapter *ApiSessionEventAdapter) AcceptApiSessionEvent(event *ApiSessionEvent) {
	if stringz.Contains(adapter.includeList, event.EventType) {
		adapter.wrapped.AcceptApiSessionEvent(event)
	}
}
