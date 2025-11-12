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

// Package env provides the router environment registry for managing xgress protocol factories.
package env

import (
	"fmt"
	"sync"

	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/router/xgress_router"
)

// Registry manages xgress protocol factories. It provides a two-phase initialization system:
// 1. Registration phase: Factory functions are registered by protocol name before initialization
// 2. Runtime phase: After initialization, factories are available for creating protocol handlers
//
// The registry is thread-safe and supports runtime factory replacement via the Replace method.
type Registry struct {
	// factories stores the initialized protocol factories, indexed by protocol name
	factories concurrenz.CopyOnWriteMap[string, xgress_router.Factory]
	// factoryF stores factory creation functions during the registration phase
	factoryF map[string]func(RouterEnv) xgress_router.Factory
	// initialized indicates whether Initialize has been called
	initialized bool
	// lock protects the registration phase and initialization
	lock sync.Mutex
}

// NewRegistry creates a new Registry instance ready for factory registration.
// The registry must be initialized with Initialize before factories can be retrieved.
func NewRegistry() *Registry {
	return &Registry{
		factoryF: make(map[string]func(RouterEnv) xgress_router.Factory),
	}
}

// Replace updates or adds a factory for the given protocol name at runtime.
// This method can be called after initialization and is thread-safe.
// It is useful for testing or dynamic factory replacement scenarios.
func (registry *Registry) Replace(name string, factory xgress_router.Factory) {
	registry.factories.Put(name, factory)
}

// Register adds a static factory for the given protocol name during the registration phase.
// This method panics if called after initialization or if the name is already registered.
// For more control over error handling, use RegisterF instead.
func (registry *Registry) Register(name string, factory xgress_router.Factory) {
	err := registry.RegisterF(name, func(routerEnv RouterEnv) xgress_router.Factory {
		return factory
	})
	if err != nil {
		panic(err)
	}
}

// RegisterF adds a factory creation function for the given protocol name during the registration phase.
// The factory function f will be called with the RouterEnv when Initialize is invoked.
// This allows factories to be created with access to the router environment.
//
// Returns an error if:
//   - The registry has already been initialized
//   - A factory with the same name is already registered
func (registry *Registry) RegisterF(name string, f func(routerEnv RouterEnv) xgress_router.Factory) error {
	registry.lock.Lock()
	defer registry.lock.Unlock()
	if registry.initialized {
		return fmt.Errorf("cannot add xgress factory '%s' after Registry has already been initialized", name)
	}
	if _, ok := registry.factoryF[name]; ok {
		return fmt.Errorf("xgress factory with name '%s' already registered", name)
	}
	registry.factoryF[name] = f
	return nil
}

// Initialize finalizes the registry by calling all registered factory functions with the provided RouterEnv.
// This method transitions the registry from the registration phase to the runtime phase.
// After initialization, no new factories can be registered via Register or RegisterF.
// Calling Initialize multiple times is safe; subsequent calls are ignored.
func (registry *Registry) Initialize(env RouterEnv) {
	registry.lock.Lock()
	defer registry.lock.Unlock()
	if !registry.initialized {
		for k, f := range registry.factoryF {
			registry.factories.Put(k, f(env))
		}
		registry.factoryF = nil
		registry.initialized = true
	}
}

// Factory retrieves the factory for the given protocol name.
// Returns an error if no factory is registered for the specified name.
// This method is thread-safe and can be called concurrently after initialization.
func (registry *Registry) Factory(name string) (xgress_router.Factory, error) {
	if factory := registry.factories.Get(name); factory != nil {
		return factory, nil
	} else {
		return nil, fmt.Errorf("xgress factory [%s] not found", name)
	}
}

// List returns a slice of all registered protocol names.
// This method is thread-safe and can be called concurrently.
func (registry *Registry) List() []string {
	names := make([]string, 0)
	for k := range registry.factories.AsMap() {
		names = append(names, k)
	}
	return names
}

// Debug returns a string representation of all registered protocol names.
// The format is "{ name1 name2 ... }" and is useful for logging and debugging.
func (registry *Registry) Debug() string {
	out := "{"
	for _, name := range registry.List() {
		out += " " + name
	}
	out += " }"
	return out
}
