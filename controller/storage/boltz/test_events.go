package boltz

import (
	"github.com/kataras/go-events"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"time"
)

const (
	TestEntityTypeChild  = "child"
	TestEntityTypeParent = "parent"
)

func NewTestEventChecker(req *require.Assertions) *TestEventChecker {
	return &TestEventChecker{
		Assertions: req,
		eventC:     make(chan *TestEvent, 10),
		errC:       make(chan error, 10),
	}
}

type TestEventChecker struct {
	*require.Assertions
	errC   chan error
	eventC chan *TestEvent
}

func (self *TestEventChecker) AddHandlers(store CrudStore) {
	for _, eventType := range []events.EventName{EventCreate, EventUpdate, EventDelete} {
		entityType := TestEntityTypeParent
		if store.IsChildStore() {
			entityType = TestEntityTypeChild
		}
		store.AddListener(eventType, self.newHandler(entityType, eventType).accept)
	}
}

func (self *TestEventChecker) newHandler(entityType string, eventType events.EventName) *testEventHandler {
	return &testEventHandler{
		entityType:   entityType,
		eventType:    eventType,
		eventChecker: self,
	}
}

func (self *TestEventChecker) RequireEvent(entityType string, entity Entity, eventType events.EventName) Entity {
	select {
	case event := <-self.eventC:
		self.NotNil(event)
		self.Equal(entityType, event.EntityType)
		self.Equal(entity.GetId(), event.Entity.GetId())
		self.Equal(eventType, event.eventType)
		return event.Entity
	case err := <-self.errC:
		self.NoError(err)
	case <-time.After(time.Second):
		self.FailNow("timed out waiting for event")
	}
	return nil
}

func (self *TestEventChecker) RequireNoEvent() {
	select {
	case event := <-self.eventC:
		self.Nil(event, "no event expected")
	case err := <-self.errC:
		self.NoError(err)
	case <-time.After(100 * time.Millisecond):
	}
}

type testEventHandler struct {
	entityType   string
	eventType    events.EventName
	eventChecker *TestEventChecker
}

func (self *testEventHandler) accept(i ...interface{}) {
	var entity Entity
	if len(i) != 1 {
		self.eventChecker.errC <- errors.Errorf("expected 1 entity, got %v", len(i))
	}
	ok := false
	entity, ok = i[0].(Entity)

	if !ok {
		self.eventChecker.errC <- errors.Errorf("expected Entity, got %T", i[0])
		return
	}
	self.eventChecker.eventC <- &TestEvent{
		EntityType: self.entityType,
		Entity:     entity,
		eventType:  self.eventType,
	}
}

type TestEvent struct {
	EntityType string
	Entity     Entity
	eventType  events.EventName
}
