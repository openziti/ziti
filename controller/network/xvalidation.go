package network

import (
	"github.com/openziti/foundation/identity/identity"
	"sync"
	"sync/atomic"
)

type xvalidators struct {
	newSessionValidators    *copyOnWriteNewSessionValidatorMap
	newTerminatorValidators *copyOnWriteNewTerminatorValidatorMap
}

type NewSessionValidator interface {
	ValidateSession(sourceRouter *Router, clientId *identity.TokenId, service *Service, data []byte) error
}

type NewTerminatorValidator interface {
	ValidateTerminator(terminator *Terminator, data []byte) error
}

type copyOnWriteNewSessionValidatorMap struct {
	value atomic.Value
	lock  sync.Mutex
}

func (m *copyOnWriteNewSessionValidatorMap) put(key string, value NewSessionValidator) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var current = m.value.Load().(map[string]NewSessionValidator)
	mapCopy := map[string]NewSessionValidator{}
	for k, v := range current {
		mapCopy[k] = v
	}
	mapCopy[key] = value
	m.value.Store(mapCopy)
}

func (m *copyOnWriteNewSessionValidatorMap) get(key string) NewSessionValidator {
	var current = m.value.Load().(map[string]NewSessionValidator)
	return current[key]
}

type copyOnWriteNewTerminatorValidatorMap struct {
	value atomic.Value
	lock  sync.Mutex
}

func (m *copyOnWriteNewTerminatorValidatorMap) put(key string, value NewTerminatorValidator) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var current = m.value.Load().(map[string]NewTerminatorValidator)
	mapCopy := map[string]NewTerminatorValidator{}
	for k, v := range current {
		mapCopy[k] = v
	}
	mapCopy[key] = value
	m.value.Store(mapCopy)
}

func (m *copyOnWriteNewTerminatorValidatorMap) get(key string) NewTerminatorValidator {
	var current = m.value.Load().(map[string]NewTerminatorValidator)
	return current[key]
}
