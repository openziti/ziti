package xtv

import (
	"github.com/michaelquigley/pfxlog"
	"sync"
	"sync/atomic"
)

func init() {
	globalRegistry = &registry{
		validators: copyOnWriteValidatorMap{
			value: &atomic.Value{},
			lock:  &sync.Mutex{},
		},
		bindings: copyOnWriteValidatorMap{
			value: &atomic.Value{},
			lock:  &sync.Mutex{},
		},
		mappings: copyOnWriteStringStringMap{
			value: &atomic.Value{},
			lock:  &sync.Mutex{},
		},
		defaultValidator: DefaultValidator{},
	}

	globalRegistry.validators.value.Store(map[string]Validator{})
	globalRegistry.bindings.value.Store(map[string]Validator{})
	globalRegistry.mappings.value.Store(map[string]string{})

}

func GetRegistry() Registry {
	return globalRegistry
}

var globalRegistry *registry

type registry struct {
	validators       copyOnWriteValidatorMap
	bindings         copyOnWriteValidatorMap
	mappings         copyOnWriteStringStringMap
	defaultValidator Validator
}

func (r *registry) RegisterBinding(binding, validatorId string) {
	r.mappings.put(binding, validatorId)
}

func (r *registry) RegisterValidator(id string, validator Validator) {
	r.validators.put(id, validator)
}

func (r *registry) GetValidatorForBinding(binding string) Validator {
	validator := r.bindings.get(binding)
	if validator == nil {
		pfxlog.Logger().Debugf("using default validator for binding %v", binding)
		validator = r.defaultValidator
		r.bindings.put(binding, validator)
	}
	return validator
}

type copyOnWriteValidatorMap struct {
	value *atomic.Value
	lock  *sync.Mutex
}

func (m *copyOnWriteValidatorMap) put(key string, value Validator) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var current = m.value.Load().(map[string]Validator)
	mapCopy := map[string]Validator{}
	for k, v := range current {
		mapCopy[k] = v
	}
	mapCopy[key] = value
	m.value.Store(mapCopy)
}

func (m *copyOnWriteValidatorMap) get(key string) Validator {
	var current = m.value.Load().(map[string]Validator)
	return current[key]
}

type copyOnWriteStringStringMap struct {
	value *atomic.Value
	lock  *sync.Mutex
}

func (m *copyOnWriteStringStringMap) put(key string, value string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var current = m.value.Load().(map[string]string)
	mapCopy := map[string]string{}
	for k, v := range current {
		mapCopy[k] = v
	}
	mapCopy[key] = value
	m.value.Store(mapCopy)
}

func (m *copyOnWriteStringStringMap) get(key string) string {
	var current = m.value.Load().(map[string]string)
	return current[key]
}
