/*
	Copyright NetFoundry, Inc.

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

package xt

import (
	"github.com/openziti/foundation/storage/boltz"
	"sync"
	"sync/atomic"
)

func init() {
	globalRegistry = &defaultRegistry{
		factories: &copyOnWriteFactoryMap{
			value: &atomic.Value{},
			lock:  &sync.Mutex{},
		},
		strategies: &copyOnWriteStrategyMap{
			value: &atomic.Value{},
			lock:  &sync.Mutex{},
		},
		lock: &sync.Mutex{},
	}

	globalRegistry.factories.value.Store(map[string]Factory{})
	globalRegistry.strategies.value.Store(map[string]Strategy{})
}

func GlobalRegistry() Registry {
	return globalRegistry
}

var globalRegistry *defaultRegistry

type defaultRegistry struct {
	factories  *copyOnWriteFactoryMap
	strategies *copyOnWriteStrategyMap
	lock       *sync.Mutex
}

func (registry *defaultRegistry) RegisterFactory(factory Factory) {
	registry.factories.put(factory.GetStrategyName(), factory)
}

func (registry *defaultRegistry) GetStrategy(name string) (Strategy, error) {
	result := registry.strategies.get(name)
	if result == nil {
		registry.lock.Lock()
		defer registry.lock.Unlock()
		result = registry.strategies.get(name)
		if result != nil {
			return result, nil
		}

		factory := registry.factories.get(name)
		if factory == nil {
			return nil, boltz.NewNotFoundError("terminatorStrategy", "name", name)
		}

		result = factory.NewStrategy()
		registry.strategies.put(factory.GetStrategyName(), result)
	}

	return result, nil
}

type copyOnWriteFactoryMap struct {
	value *atomic.Value
	lock  *sync.Mutex
}

func (m *copyOnWriteFactoryMap) put(key string, value Factory) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var current = m.value.Load().(map[string]Factory)
	mapCopy := map[string]Factory{}
	for k, v := range current {
		mapCopy[k] = v
	}
	mapCopy[key] = value
	m.value.Store(mapCopy)
}

func (m *copyOnWriteFactoryMap) get(key string) Factory {
	var current = m.value.Load().(map[string]Factory)
	return current[key]
}

type copyOnWriteStrategyMap struct {
	value *atomic.Value
	lock  *sync.Mutex
}

func (m *copyOnWriteStrategyMap) put(key string, value Strategy) {
	m.lock.Lock()
	defer m.lock.Unlock()

	var current = m.value.Load().(map[string]Strategy)
	mapCopy := map[string]Strategy{}
	for k, v := range current {
		mapCopy[k] = v
	}
	mapCopy[key] = value
	m.value.Store(mapCopy)
}

func (m *copyOnWriteStrategyMap) get(key string) Strategy {
	var current = m.value.Load().(map[string]Strategy)
	return current[key]
}
