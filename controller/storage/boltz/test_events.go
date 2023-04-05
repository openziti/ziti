package boltz

import (
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

func (self *TestEventChecker) AddHandlers(store CrudBaseStore) {
	for _, eventType := range []EntityEventType{EntityCreated, EntityUpdated, EntityDeleted} {
		entityType := TestEntityTypeParent
		if store.IsChildStore() {
			entityType = TestEntityTypeChild
		}
		store.AddListener(self.newHandler(entityType, eventType).accept, eventType)
	}
}

func (self *TestEventChecker) newHandler(entityType string, eventType EntityEventType) *testEventHandler {
	return &testEventHandler{
		entityType:   entityType,
		eventType:    eventType,
		eventChecker: self,
	}
}

func (self *TestEventChecker) RequireEvent(entityType string, entity Entity, eventType EntityEventType) Entity {
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
	eventType    EntityEventType
	eventChecker *TestEventChecker
}

func (self *testEventHandler) accept(entity Entity) {
	self.eventChecker.eventC <- &TestEvent{
		EntityType: self.entityType,
		Entity:     entity,
		eventType:  self.eventType,
	}
}

type TestEvent struct {
	EntityType string
	Entity     Entity
	eventType  EntityEventType
}
